// The adapter side of benchwrap's protocol (harness/benchwrap): the
// pattern and the iteration counts the wrapper passes, the messages on
// file descriptor 3, and the measuring loop with the kit's policy.
#pragma once

#include <algorithm>
#include <chrono>
#include <cstdint>
#include <optional>
#include <regex>
#include <string>
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

    // Measures one leaf: the untimed warm-up calls (two when the operation
    // allocates), then the fixed iterations, or a count grown as Go's
    // testing does until a batch reaches the benchtime. The results of a
    // batch are destroyed between batches with the counters paused.
    template <class F>
    void run(const Leaf& l, F&& f) {
        std::size_t fixed = 0;
        for (const auto& [op, n] : iters_) {
            if (op == l.op) fixed = n;
        }
        std::size_t n = fixed > 0 ? fixed : std::max<std::size_t>(fixed_, 1);
        auto [b0, c0] = allocs();
        { auto r = f(); (void)r; }
        auto [b1, c1] = allocs();
        int warm = 1;
        if (b1 != b0) {
            { auto r = f(); (void)r; }
            warm = 2;
        }
        for (;;) {
            Timed t = timed(l, n, f);
            if (target_ && fixed == 0 && t.elapsed < *target_) {
                double ratio = std::chrono::duration<double>(*target_).count() /
                               std::max(std::chrono::duration<double>(t.elapsed).count(), 1e-9);
                auto next = static_cast<std::size_t>(static_cast<double>(n) * ratio * 1.2);
                n = roundUp(std::max(next, n + 1));
                continue;
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

    template <class F>
    Timed timed(const Leaf& l, std::size_t n, F& f) {
        using R = decltype(f());
        using clock = std::chrono::steady_clock;
        std::vector<R> kept;
        kept.reserve(std::min<std::size_t>(n, 1 << 16));
        std::size_t every = 1;
        std::chrono::nanoseconds elapsed{0};
        std::uint64_t sumBytes = 0, sumCount = 0;
        // The messages are built after the counters are read, so that
        // their strings are not the operation's allocations.
        send("begin " + l.name());
        auto [bytes0, count0] = allocs();
        auto start = clock::now();
        for (std::size_t i = 0; i < n; i++) {
            kept.push_back(f());
            bool last = i + 1 == n;
            bool batch = i == 0 || (i + 1) % every == 0;
            if (!batch && !last) continue;
            elapsed += clock::now() - start;
            auto [b1, c1] = allocs();
            sumBytes += b1 - bytes0;
            sumCount += c1 - count0;
            if (i == 0) {
                every = sumBytes == 0 ? SIZE_MAX : sumBytes < kDropBatch ? kDropBatch / sumBytes : 1;
            }
            if (!last) {
                send("pause");
                kept.clear();
                send("resume");
                auto [b, c] = allocs();
                bytes0 = b;
                count0 = c;
                start = clock::now();
            }
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
