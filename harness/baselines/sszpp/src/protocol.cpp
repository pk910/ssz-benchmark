#include "protocol.hpp"

#include <fcntl.h>
#include <sys/syscall.h>
#include <unistd.h>

#include <cstdlib>
#include <cstring>
#include <sstream>

namespace {

std::string env(const char* name, const char* fallback) {
    const char* v = std::getenv(name);
    return v ? v : fallback;
}

std::vector<std::string> split(const std::string& s, char sep) {
    std::vector<std::string> out;
    std::string cur;
    for (char c : s) {
        if (c == sep) {
            out.push_back(cur);
            cur.clear();
        } else {
            cur += c;
        }
    }
    out.push_back(cur);
    return out;
}

std::string trim(const std::string& s) {
    auto b = s.find_first_not_of(" \t");
    if (b == std::string::npos) return "";
    auto e = s.find_last_not_of(" \t");
    return s.substr(b, e - b + 1);
}

// Go's duration syntax, the units the runner uses.
std::optional<std::chrono::nanoseconds> parseDuration(const std::string& s) {
    auto t = trim(s);
    auto i = t.find_first_of("abcdefghijklmnopqrstuvwxyzµ");
    if (i == std::string::npos || i == 0) return std::nullopt;
    double v = std::strtod(t.substr(0, i).c_str(), nullptr);
    std::string unit = t.substr(i);
    double ns;
    if (unit == "ns") {
        ns = v;
    } else if (unit == "us" || unit == "µs") {
        ns = v * 1e3;
    } else if (unit == "ms") {
        ns = v * 1e6;
    } else if (unit == "s") {
        ns = v * 1e9;
    } else if (unit == "m") {
        ns = v * 60e9;
    } else {
        return std::nullopt;
    }
    return std::chrono::nanoseconds(static_cast<std::int64_t>(ns));
}

}  // namespace

Session::Session() {
    fd_ = fcntl(3, F_GETFD) != -1 ? 3 : 2;
    for (const auto& p : split(env("BENCH_PATTERN", "."), '/')) {
        pattern_.emplace_back(p, std::regex::ECMAScript);
    }
    for (const auto& part : split(env("BENCH_ITERS", ""), ',')) {
        auto eq = part.find('=');
        if (eq == std::string::npos) continue;
        auto n = std::strtoull(trim(part.substr(eq + 1)).c_str(), nullptr, 10);
        if (n > 0) iters_.emplace_back(trim(part.substr(0, eq)), n);
    }
    std::string benchTime = env("BENCH_TIME", "1x");
    if (!benchTime.empty() && benchTime.back() == 'x') {
        fixed_ = std::strtoull(benchTime.c_str(), nullptr, 10);
        if (fixed_ == 0) fixed_ = 1;
    } else {
        fixed_ = 0;
        target_ = parseDuration(benchTime);
    }
}

bool Session::matches(const Leaf& l) const {
    const std::string name[] = {"BenchmarkReal", l.engine, l.object, l.op};
    for (std::size_t i = 0; i < pattern_.size() && i < 4; i++) {
        if (!std::regex_search(name[i], pattern_[i])) return false;
    }
    return true;
}

void Session::send(const std::string& msg) {
    std::string line = msg + "\n";
    const char* p = line.data();
    std::size_t left = line.size();
    while (left > 0) {
        ssize_t n = write(fd_, p, left);
        if (n <= 0) return;
        p += n;
        left -= static_cast<std::size_t>(n);
    }
}

void Session::thread() { send("thread " + std::to_string(syscall(SYS_gettid))); }

void Session::fail(const Leaf& l, const std::string& msg) {
    std::string flat = msg;
    for (char& c : flat) {
        if (c == '\n') c = ' ';
    }
    send("fail " + l.name() + " " + flat);
}

void Session::skip(const Leaf& l) { send("skip " + l.name()); }

// roundUp rounds a count up to 1, 2, 5 times a power of ten, as Go's
// testing grows its iteration counts.
std::size_t Session::roundUp(std::size_t n) {
    std::size_t base = 1;
    while (base * 10 <= n) base *= 10;
    if (n <= base) return base;
    if (n <= 2 * base) return 2 * base;
    if (n <= 5 * base) return 5 * base;
    return 10 * base;
}
