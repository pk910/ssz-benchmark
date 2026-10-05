// The adapter side of benchwrap's protocol (harness/benchwrap): the
// pattern and the iteration counts the wrapper passes, the messages on
// file descriptor 3, and the measuring loop with the kit's policy.
#pragma once

#include <algorithm>
#include <chrono>
#include <cstdint>
#include <optional>
#include <regex>
#include <span>
#include <string>
#include <tuple>
#include <type_traits>
#include <utility>
#include <vector>

// The bytes and the number of allocations since the start, from the
// counting operator new of main.cpp.
std::pair<std::uint64_t, std::uint64_t> allocs();

struct Leaf {
    std::string engine, object, op;
    std::string name() const { return engine + "/" + object + "/" + op; }
};

class Session {
   public:
    // Takes the pipe on file descriptor 3 (stderr when run by hand) and the
    // environment of the wrapper.
    Session();

    // Go testing's matching: each element of the pattern against the element
    // of "BenchmarkReal/<Engine>/<Object>/<Op>" at its position.
    bool matches(const Leaf& l) const;

    void thread();
    void fail(const Leaf& l, const std::string& msg);
    void skip(const Leaf& l);

    // Measures one leaf. One call of f(out) is one iteration: it pushes
    // its results (perIter of them, one per item of the payload) onto
    // out, which has room for them, so that the loop allocates nothing
    // the operation does not. The results of a batch are kept and
    // destroyed between batches with the counters paused, as the kit's
    // garbage waits for a collection between batches; a result without
    // a destructor (a size, a root) is overwritten every iteration, as
    // the kit overwrites its variable. First the untimed warm-up calls
    // (two when the operation allocates), then the fixed iterations, or a
    // count grown as Go's testing does until a batch reaches the
    // benchtime. After the loop, check(last) sees the results of the last
    // iteration and returns the failure message, empty when they are
    // right: the leaf then fails as the kit's does, with no figures.
    template <class R, class F, class C>
    void run(const Leaf& l, std::size_t perIter, F&& f, C&& check) {
        std::size_t fixed = 0;
        for (const auto& [op, n] : iters_) {
            if (op == l.op) fixed = n;
        }
        std::size_t n = fixed > 0 ? fixed : std::max<std::size_t>(fixed_, 1);
        std::vector<R> kept;
        kept.reserve(perIter);
        auto [b0, c0] = allocs();
        f(kept);
        auto [b1, c1] = allocs();
        int warm = 1;
        if (b1 != b0) {
            kept.clear();
            f(kept);
            warm = 2;
        }
        for (;;) {
            Timed t = timed(l, n, perIter, f, kept);
            if (target_ && fixed == 0 && t.elapsed < *target_) {
                // Go's testing: the count the benchtime predicts, a fifth
                // more, at most a hundredfold and at least one more,
                // rounded up to 1, 2, 5 times a power of ten.
                double ratio = std::chrono::duration<double>(*target_).count() /
                               std::max(std::chrono::duration<double>(t.elapsed).count(), 1e-9);
                auto next = static_cast<std::size_t>(static_cast<double>(n) * ratio * 1.2);
                next = std::min(next, 100 * n);
                next = std::max(next, n + 1);
                next = std::min<std::size_t>(next, 1000000000);
                n = roundUp(next);
                continue;
            }
            std::span<const R> all(kept);
            std::string msg = check(all.last(std::min(perIter, all.size())));
            if (!msg.empty()) {
                fail(l, msg);
                return;
            }
            send("end " + l.name() + " iters=" + std::to_string(t.iters) + " ns=" +
                 std::to_string(std::chrono::duration_cast<std::chrono::nanoseconds>(t.elapsed).count()) +
                 " bytes=" + std::to_string(t.bytes) + " allocs=" + std::to_string(t.count) +
                 " warmup=" + std::to_string(warm));
            return;
        }
    }

   private:
    struct Timed {
        std::size_t iters;
        std::chrono::nanoseconds elapsed;
        std::uint64_t bytes, count;
    };

    // A batch of iterations may leave this much behind before its results
    // are destroyed between batches, as the kit's collections are spaced.
    static constexpr std::uint64_t kDropBatch = std::uint64_t{256} << 20;

    // The timed loop of n iterations: the clock and the allocation
    // counters run across the iterations and pause only around the
    // destruction of a batch's results, as the kit's do around its
    // collections. A batch is as many iterations as keep the garbage
    // under kDropBatch, judged from the first iteration's allocation.
    template <class R, class F>
    Timed timed(const Leaf& l, std::size_t n, std::size_t perIter, F& f, std::vector<R>& kept) {
        using clock = std::chrono::steady_clock;
        constexpr bool trivial = std::is_trivially_destructible_v<R>;
        // Room for the results before the counters start: up to 65536
        // iterations until the first one tells how long a batch is.
        kept.clear();
        kept.reserve((trivial ? 1 : std::min<std::size_t>(n, 1 << 16)) * perIter);
        std::size_t every = 1;
        std::chrono::nanoseconds elapsed{0};
        std::uint64_t sumBytes = 0, sumCount = 0, bytes0 = 0, count0 = 0;
        // The messages are built after the counters are read, so that
        // their strings are not the operation's allocations.
        send("begin " + l.name());
        std::tie(bytes0, count0) = allocs();
        auto start = clock::now();
        for (std::size_t i = 0; i < n; i++) {
            if constexpr (trivial) kept.clear();
            f(kept);
            // The results are used: nothing of the operation is dead code.
            asm volatile("" : : "r"(kept.data()) : "memory");
            bool last = i + 1 == n;
            bool batch = i == 0 || (i + 1) % every == 0;
            if (!batch && !last) continue;
            elapsed += clock::now() - start;
            auto [b1, c1] = allocs();
            sumBytes += b1 - bytes0;
            sumCount += c1 - count0;
            send("pause");
            if (last) break;
            if (i == 0) {
                every = sumBytes == 0 ? SIZE_MAX : sumBytes < kDropBatch ? kDropBatch / sumBytes : 1;
            }
            if constexpr (!trivial) {
                kept.clear();
                if (i == 0) kept.reserve(std::min(n, every) * perIter);
            }
            send("resume");
            std::tie(bytes0, count0) = allocs();
            start = clock::now();
        }
        return Timed{n, elapsed, sumBytes, sumCount};
    }

    static std::size_t roundUp(std::size_t n);
    void send(const std::string& msg);

    int fd_;
    std::vector<std::regex> pattern_;
    std::vector<std::pair<std::string, std::size_t>> iters_;
    std::size_t fixed_ = 1;
    std::optional<std::chrono::nanoseconds> target_;
};
