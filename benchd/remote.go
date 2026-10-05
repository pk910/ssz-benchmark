package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The runner API: what a worker machine calls on the controller. Every
// request carries "Authorization: Bearer <token>".
//
//	POST /runner/claim            {Runner}                      -> job or null
//	POST /runner/status           liveStatus
//	GET  /runner/leaves?harness=  -> []leaf
//	POST /runner/leaves?harness=  []leaf
//	GET  /runner/iters            -> {key: n}
//	POST /runner/iters            {Engine,Object,Op,Iters}
//	POST /runner/job/<id>/started {GoVersion,Harness}
//	POST /runner/job/<id>/log     {Line}
//	POST /runner/job/<id>/reset
//	POST /runner/job/<id>/samples []sample
//	POST /runner/job/<id>/finish  {Passes,Error,Seconds}
//	POST /runner/job/<id>/interrupted

// remoteStore is the jobStore of a worker: HTTP calls to the controller.
type remoteStore struct {
	base   string
	token  string
	client *http.Client
}

func newRemoteStore(base, token string) *remoteStore {
	return &remoteStore{base: strings.TrimRight(base, "/"), token: token, client: &http.Client{Timeout: 60 * time.Second}}
}

func (s *remoteStore) call(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
		}
		req, err := http.NewRequest(method, s.base+path, body)
		if err != nil {
			return err
		}
		if body != nil {
			if seeker, ok := body.(io.Seeker); ok {
				_, _ = seeker.Seek(0, io.SeekStart)
			}
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+s.token)
		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadRequest {
				return lastErr
			}
			continue
		}
		if out != nil && len(data) > 0 {
			return json.Unmarshal(data, out)
		}
		return nil
	}
	return lastErr
}

func (s *remoteStore) claim(runner string) (*job, error) {
	var j *job
	err := s.call("POST", "/runner/claim", map[string]string{"Runner": runner}, &j)
	return j, err
}

func (s *remoteStore) started(id int64, goVersion, harness string) error {
	return s.call("POST", fmt.Sprintf("/runner/job/%d/started", id), map[string]string{"GoVersion": goVersion, "Harness": harness}, nil)
}

func (s *remoteStore) publish(ls liveStatus) error {
	return s.call("POST", "/runner/status", ls, nil)
}

func (s *remoteStore) log(id int64, line string) {
	_ = s.call("POST", fmt.Sprintf("/runner/job/%d/log", id), map[string]string{"Line": line}, nil)
}

func (s *remoteStore) harnessLeaves(hash string) ([]leaf, error) {
	var leaves []leaf
	err := s.call("GET", "/runner/leaves?harness="+hash, nil, &leaves)
	return leaves, err
}

func (s *remoteStore) setHarnessLeaves(hash string, leaves []leaf) error {
	return s.call("POST", "/runner/leaves?harness="+hash, leaves, nil)
}

func (s *remoteStore) benchIters() (map[string]int, error) {
	out := map[string]int{}
	err := s.call("GET", "/runner/iters", nil, &out)
	return out, err
}

func (s *remoteStore) setBenchIters(l leaf, n int) error {
	return s.call("POST", "/runner/iters", struct {
		leaf
		Iters int
	}{l, n}, nil)
}

func (s *remoteStore) resetSamples(id int64) error {
	return s.call("POST", fmt.Sprintf("/runner/job/%d/reset", id), nil, nil)
}

// baseRuns: a worker measures both sides itself.
func (s *remoteStore) baseRuns(*job, []string) (map[string]sample, error) { return nil, nil }

func (s *remoteStore) addBuilds(facts []buildFact) error {
	if len(facts) == 0 {
		return nil
	}
	return s.call("POST", fmt.Sprintf("/runner/job/%d/builds", facts[0].JobID), facts, nil)
}

func (s *remoteStore) addSamples(samples []sample) error {
	if len(samples) == 0 {
		return nil
	}
	return s.call("POST", fmt.Sprintf("/runner/job/%d/samples", samples[0].JobID), samples, nil)
}

func (s *remoteStore) finish(id int64, passes int, errText string, seconds float64) error {
	return s.call("POST", fmt.Sprintf("/runner/job/%d/finish", id), map[string]any{"Passes": passes, "Error": errText, "Seconds": seconds}, nil)
}

// A worker does not stop a refinement run for a waiting job.
func (s *remoteStore) waiting(string) (int64, error)   { return 0, nil }
func (s *remoteStore) giveWay(int64, int64, int) error { return nil }

func (s *remoteStore) interrupted(id int64) error {
	return s.call("POST", fmt.Sprintf("/runner/job/%d/interrupted", id), nil, nil)
}

// runnerAPI serves the runner API on the controller over its local store.
type runnerAPI struct {
	store *localStore
	token string
}

func (a *runnerAPI) register(mux *http.ServeMux) {
	mux.HandleFunc("/runner/", a.handle)
}

func (a *runnerAPI) handle(rw http.ResponseWriter, req *http.Request) {
	if a.token == "" {
		http.Error(rw, "runner API disabled: no RUNNER_TOKEN", http.StatusForbidden)
		return
	}
	got := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) != 1 {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(req.URL.Path, "/runner/")
	decode := func(v any) bool {
		if err := json.NewDecoder(io.LimitReader(req.Body, 64<<20)).Decode(v); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return false
		}
		return true
	}
	reply := func(v any) {
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(v)
	}
	fail := func(err error) {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}
	switch {
	case path == "claim" && req.Method == http.MethodPost:
		var in struct{ Runner string }
		if !decode(&in) {
			return
		}
		if in.Runner == "" || in.Runner == a.store.local {
			http.Error(rw, "runner name required, and not the controller's", http.StatusBadRequest)
			return
		}
		j, err := a.store.claim(in.Runner)
		if err != nil {
			fail(err)
			return
		}
		reply(j)
	case path == "status" && req.Method == http.MethodPost:
		var ls liveStatus
		if !decode(&ls) {
			return
		}
		if ls.Runner == "" {
			http.Error(rw, "runner name required", http.StatusBadRequest)
			return
		}
		ls.Updated = time.Now()
		if err := a.store.publish(ls); err != nil {
			fail(err)
			return
		}
		reply(true)
	case path == "leaves" && req.Method == http.MethodGet:
		leaves, err := a.store.harnessLeaves(req.URL.Query().Get("harness"))
		if err != nil {
			fail(err)
			return
		}
		if leaves == nil {
			leaves = []leaf{}
		}
		reply(leaves)
	case path == "leaves" && req.Method == http.MethodPost:
		var leaves []leaf
		if !decode(&leaves) {
			return
		}
		if err := a.store.setHarnessLeaves(req.URL.Query().Get("harness"), leaves); err != nil {
			fail(err)
			return
		}
		reply(true)
	case path == "iters" && req.Method == http.MethodGet:
		iters, err := a.store.benchIters()
		if err != nil {
			fail(err)
			return
		}
		reply(iters)
	case path == "iters" && req.Method == http.MethodPost:
		var in struct {
			leaf
			Iters int
		}
		if !decode(&in) {
			return
		}
		if err := a.store.setBenchIters(in.leaf, in.Iters); err != nil {
			fail(err)
			return
		}
		reply(true)
	case strings.HasPrefix(path, "job/") && req.Method == http.MethodPost:
		idStr, action, _ := strings.Cut(strings.TrimPrefix(path, "job/"), "/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(rw, "bad job id", http.StatusBadRequest)
			return
		}
		switch action {
		case "started":
			var in struct{ GoVersion, Harness string }
			if !decode(&in) {
				return
			}
			err = a.store.started(id, in.GoVersion, in.Harness)
		case "log":
			var in struct{ Line string }
			if !decode(&in) {
				return
			}
			a.store.log(id, in.Line)
		case "reset":
			err = a.store.resetSamples(id)
		case "samples":
			var samples []sample
			if !decode(&samples) {
				return
			}
			for i := range samples {
				samples[i].JobID = id
			}
			err = a.store.addSamples(samples)
		case "builds":
			var facts []buildFact
			if !decode(&facts) {
				return
			}
			for i := range facts {
				facts[i].JobID = id
			}
			err = a.store.addBuilds(facts)
		case "finish":
			var in struct {
				Passes  int
				Error   string
				Seconds float64
			}
			if !decode(&in) {
				return
			}
			err = a.store.finish(id, in.Passes, in.Error, in.Seconds)
		case "interrupted":
			err = a.store.interrupted(id)
		default:
			http.NotFound(rw, req)
			return
		}
		if err != nil {
			fail(err)
			return
		}
		reply(true)
	default:
		http.NotFound(rw, req)
	}
}
