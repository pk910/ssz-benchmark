// The adapter of sszpp for benchwrap: the harness objects decoded into
// the generated C++ types (gen_fulu.hpp, one namespace per preset), every
// operation measured with the kit's policy, the figures sent over the
// protocol (harness/benchwrap).
//
// Operations: Unmarshal (ssz::deserialize<T*>, the object on the heap as
// the library's own benchmark decodes a state), SizeSSZ (ssz_size),
// Marshal (ssz::serialize into a new buffer), MarshalTo (ssz::serialize
// through an iterator into one buffer kept across iterations and reused
// for every item of a set; the library ORs bit fields into the
// destination, so the region is sized with ssz_size and zeroed before
// every object, which is what a user of this entry point must do) and
// HashTreeRoot (ssz::hash_tree_root on one thread; the library splits
// large vectors over std::async threads when asked for more; it caches
// nothing). Every leaf checks the results of its last iteration after the
// loop as the kit does: decoded values must encode back to the input,
// encoded bytes must equal the input, roots must equal the stored root.
//
// Objects of the fulu launcher: the state, the block, the block set, the
// minimal-preset state. The minimal-preset block is left out: its
// attestations carry aggregation bits at the limit (8192 bits in 1025
// bytes), and the library's bitlist deserialization refuses a bitlist
// whose byte count times eight exceeds the limit, which counts the byte
// of the length bit (lists.hpp, "byte slice larger than list limit"). The
// object is decoded all the same, and left out, with a note on stderr,
// only when the decode fails so. The Gloas types are progressive
// containers and lists, which sszpp has no notion of: no gloas launcher.
//
// Memory: a counting operator new gives the bytes and allocations per
// operation. Freeing is not part of an operation: as the kit collects
// between batches with the clock stopped, the results of a batch are
// destroyed between batches with the counters paused. The heap stays
// mapped as the kit's does (GOGC=off, the runtime keeps its pages): glibc
// would otherwise unmap every large block on free and trim the heap when
// a batch's results are destroyed, and the next batch would pay the
// kernel for every page again (half of a block's Marshal time, a third of
// a state decode), which the kit's warm-up calls exist to keep out of the
// figures; main sets malloc to serve everything from a heap it never trims.
#include <malloc.h>

#include <algorithm>
#include <atomic>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <filesystem>
#include <fstream>
#include <memory>
#include <new>
#include <span>
#include <stdexcept>
#include <string>
#include <vector>

#include "gen_fulu.hpp"
#include "protocol.hpp"

namespace {

std::atomic<std::uint64_t> allocBytes{0}, allocCount{0};

// Every allocation counts, the nothrow and the aligned forms included
// (null when out of memory; the throwing operators throw on it).
void* counted(std::size_t n) noexcept {
    allocBytes.fetch_add(n, std::memory_order_relaxed);
    allocCount.fetch_add(1, std::memory_order_relaxed);
    return std::malloc(n ? n : 1);
}

void* countedAligned(std::size_t n, std::align_val_t al) noexcept {
    allocBytes.fetch_add(n, std::memory_order_relaxed);
    allocCount.fetch_add(1, std::memory_order_relaxed);
    std::size_t a = static_cast<std::size_t>(al);
    return std::aligned_alloc(a, (n + a - 1) / a * a);
}

void* orThrow(void* p) {
    if (!p) throw std::bad_alloc();
    return p;
}

}  // namespace

void* operator new(std::size_t n) { return orThrow(counted(n)); }
void* operator new[](std::size_t n) { return orThrow(counted(n)); }
void* operator new(std::size_t n, const std::nothrow_t&) noexcept { return counted(n); }
void* operator new[](std::size_t n, const std::nothrow_t&) noexcept { return counted(n); }
void* operator new(std::size_t n, std::align_val_t al) { return orThrow(countedAligned(n, al)); }
void* operator new[](std::size_t n, std::align_val_t al) { return orThrow(countedAligned(n, al)); }
void* operator new(std::size_t n, std::align_val_t al, const std::nothrow_t&) noexcept { return countedAligned(n, al); }
void* operator new[](std::size_t n, std::align_val_t al, const std::nothrow_t&) noexcept { return countedAligned(n, al); }
void operator delete(void* p) noexcept { std::free(p); }
void operator delete[](void* p) noexcept { std::free(p); }
void operator delete(void* p, std::size_t) noexcept { std::free(p); }
void operator delete[](void* p, std::size_t) noexcept { std::free(p); }
void operator delete(void* p, const std::nothrow_t&) noexcept { std::free(p); }
void operator delete[](void* p, const std::nothrow_t&) noexcept { std::free(p); }
void operator delete(void* p, std::align_val_t) noexcept { std::free(p); }
void operator delete[](void* p, std::align_val_t) noexcept { std::free(p); }
void operator delete(void* p, std::size_t, std::align_val_t) noexcept { std::free(p); }
void operator delete[](void* p, std::size_t, std::align_val_t) noexcept { std::free(p); }
void operator delete(void* p, std::align_val_t, const std::nothrow_t&) noexcept { std::free(p); }
void operator delete[](void* p, std::align_val_t, const std::nothrow_t&) noexcept { std::free(p); }

std::pair<std::uint64_t, std::uint64_t> allocs() {
    return {allocBytes.load(std::memory_order_relaxed), allocCount.load(std::memory_order_relaxed)};
}

namespace {

constexpr const char* kEngine = "SszPP";
constexpr const char* kOps[] = {"Unmarshal", "SizeSSZ", "Marshal", "MarshalTo", "HashTreeRoot"};

namespace fs = std::filesystem;

// A payload: the bytes of one or more objects with their roots.
struct Payload {
    std::vector<std::pair<std::vector<std::byte>, ssz::chunk_t>> items;
};

std::vector<std::byte> readFile(const fs::path& p) {
    std::ifstream in(p, std::ios::binary);
    if (!in) throw std::runtime_error(p.string() + ": cannot open");
    std::vector<std::byte> out(fs::file_size(p));
    in.read(reinterpret_cast<char*>(out.data()), static_cast<std::streamsize>(out.size()));
    return out;
}

ssz::chunk_t readRoot(const fs::path& p) {
    std::ifstream in(p);
    std::string hex;
    in >> hex;
    if (hex.rfind("0x", 0) == 0) hex = hex.substr(2);
    if (hex.size() != 64) throw std::runtime_error(p.string() + ": not a root");
    ssz::chunk_t out{};
    for (std::size_t i = 0; i < 32; i++) {
        out[i] = static_cast<std::byte>(std::stoul(hex.substr(2 * i, 2), nullptr, 16));
    }
    return out;
}

std::string hex(const ssz::chunk_t& c) {
    static const char digits[] = "0123456789abcdef";
    std::string s = "0x";
    for (auto b : c) {
        s += digits[std::to_integer<int>(b) >> 4];
        s += digits[std::to_integer<int>(b) & 15];
    }
    return s;
}

Payload loadOne(const fs::path& dir, const std::string& name) {
    Payload p;
    p.items.emplace_back(readFile(dir / (name + ".ssz")), readRoot(dir / (name + ".root")));
    return p;
}

// The .ssz files of the directory with their roots, in name order.
Payload loadSet(const fs::path& dir, const std::string& name) {
    std::vector<fs::path> files;
    for (const auto& e : fs::directory_iterator(dir / name)) {
        if (e.path().extension() == ".ssz") files.push_back(e.path());
    }
    std::sort(files.begin(), files.end());
    if (files.empty()) throw std::runtime_error((dir / name).string() + ": no files");
    Payload p;
    for (const auto& f : files) {
        fs::path root = f;
        root.replace_extension(".root");
        p.items.emplace_back(readFile(f), readRoot(root));
    }
    return p;
}

// The leaves of an object the pattern selects.
std::vector<Leaf> leaves(const Session& s, const std::string& object) {
    std::vector<Leaf> out;
    for (const char* op : kOps) {
        Leaf l{kEngine, object, op};
        if (s.matches(l)) out.push_back(l);
    }
    return out;
}

template <class T>
using RootOf = ssz::chunk_t (*)(const T&);

// Decodes every item of the payload and checks it as the kit does:
// re-encoded bytes equal the input, the root equals the stored one.
template <class T>
std::vector<std::unique_ptr<T>> verify(const Payload& p, RootOf<T> rootOf) {
    std::vector<std::unique_ptr<T>> out;
    for (const auto& [data, root] : p.items) {
        std::unique_ptr<T> v(ssz::deserialize<T*>(data));
        if (v->ssz_size() != data.size()) {
            throw std::runtime_error("decoded value has size " + std::to_string(v->ssz_size()) + ", input " +
                                     std::to_string(data.size()));
        }
        if (ssz::serialize(*v) != data) throw std::runtime_error("decoded value does not encode back to the input");
        ssz::chunk_t got = rootOf(*v);
        if (got != root) throw std::runtime_error("root mismatch: got " + hex(got) + " want " + hex(root));
        out.push_back(std::move(v));
    }
    return out;
}

// Measures the operations of an object; one iteration of a set runs the
// operation on every item, in order. A verification failure fails every
// leaf, unless its message holds leaveOutOn, a limitation of the library
// known for this object: the object is then left out with a note.
template <class T>
void bench(Session& s, const std::vector<Leaf>& ls, const Payload& p, RootOf<T> rootOf, const char* leaveOutOn) {
    std::vector<std::unique_ptr<T>> decoded;
    try {
        decoded = verify<T>(p, rootOf);
    } catch (const std::exception& e) {
        if (leaveOutOn && std::string(e.what()).find(leaveOutOn) != std::string::npos) {
            std::fprintf(stderr, "%s/%s: left out, the library cannot decode the object: %s\n", kEngine,
                         ls[0].object.c_str(), e.what());
            return;
        }
        for (const auto& l : ls) s.fail(l, e.what());
        return;
    }
    const auto& items = p.items;
    const std::size_t count = items.size();
    std::size_t total = 0, largest = 0;
    for (const auto& [d, _] : items) {
        total += d.size();
        largest = std::max(largest, d.size());
    }
    for (const auto& l : ls) {
        if (l.op == "Unmarshal") {
            s.run<std::unique_ptr<T>>(
                l, count, [&](auto& out) {
                    for (const auto& [d, _] : items) out.emplace_back(ssz::deserialize<T*>(d));
                },
                [&](auto last) -> std::string {
                    for (std::size_t j = 0; j < last.size(); j++) {
                        if (ssz::serialize(*last[j]) != items[j].first) {
                            return "decoded value " + std::to_string(j) + " does not encode back to the input";
                        }
                    }
                    return "";
                });
        } else if (l.op == "SizeSSZ") {
            s.run<std::size_t>(
                l, 1, [&](auto& out) {
                    std::size_t sum = 0;
                    for (const auto& v : decoded) sum += v->ssz_size();
                    out.push_back(sum);
                },
                [&](auto last) -> std::string {
                    if (last[0] != total) return "size " + std::to_string(last[0]) + " want " + std::to_string(total);
                    return "";
                });
        } else if (l.op == "Marshal") {
            s.run<std::vector<std::byte>>(
                l, count, [&](auto& out) {
                    for (const auto& v : decoded) out.push_back(ssz::serialize(*v));
                },
                [&](auto last) -> std::string {
                    for (std::size_t j = 0; j < last.size(); j++) {
                        if (last[j] != items[j].first) return "item " + std::to_string(j) + " marshal output differs";
                    }
                    return "";
                });
        } else if (l.op == "MarshalTo") {
            // One buffer of the largest object, kept across iterations and
            // reused for every item; the buffer holds the last item after
            // the loop.
            std::vector<std::byte> buf(largest);
            s.run<std::size_t>(
                l, 1, [&](auto& out) {
                    std::size_t n = 0;
                    for (const auto& v : decoded) {
                        std::size_t size = v->ssz_size();
                        std::memset(buf.data(), 0, size);
                        ssz::serialize(buf.begin(), *v);
                        n += size;
                    }
                    out.push_back(n);
                },
                [&](auto last) -> std::string {
                    const auto& d = items.back().first;
                    if (last[0] != total || !std::equal(d.begin(), d.end(), buf.begin())) {
                        return "marshalTo output differs from input";
                    }
                    return "";
                });
        } else if (l.op == "HashTreeRoot") {
            s.run<ssz::chunk_t>(
                l, count, [&](auto& out) {
                    for (const auto& v : decoded) out.push_back(rootOf(*v));
                },
                [&](auto last) -> std::string {
                    for (std::size_t j = 0; j < last.size(); j++) {
                        if (last[j] != items[j].second) {
                            return "root " + std::to_string(j) + " mismatch: got " + hex(last[j]) + " want " +
                                   hex(items[j].second);
                        }
                    }
                    return "";
                });
        } else {
            s.skip(l);
        }
    }
}

template <class T>
void one(Session& s, const std::string& object, const fs::path& dir, const std::string& file, RootOf<T> rootOf,
         const char* leaveOutOn = nullptr) {
    auto ls = leaves(s, object);
    if (ls.empty()) return;
    bench<T>(s, ls, loadOne(dir, file), rootOf, leaveOutOn);
}

template <class T>
void set(Session& s, const std::string& object, const fs::path& dir, const std::string& name, RootOf<T> rootOf) {
    auto ls = leaves(s, object);
    if (ls.empty()) return;
    bench<T>(s, ls, loadSet(dir, name), rootOf, nullptr);
}

template <class T>
ssz::chunk_t own(const T& v) {
    return ssz::hash_tree_root(v, 1);
}

template <class T>
ssz::chunk_t message(const T& v) {
    return ssz::hash_tree_root(v.Message, 1);
}

// The message of the library's refusal of a bitlist at its limit, which
// leaves the minimal-preset block out (see the top of the file).
constexpr const char* kBitlistAtLimit = "byte slice larger than list limit";

}  // namespace

int main(int argc, char** argv) {
    // A heap that is never trimmed and serves the large blocks too (see
    // the top of the file).
    mallopt(M_MMAP_MAX, 0);
    mallopt(M_TRIM_THRESHOLD, -1);
    mallopt(M_TOP_PAD, 64 << 20);
    std::string fork = "fulu";
    for (int i = 1; i + 1 < argc; i++) {
        if (std::strcmp(argv[i], "--fork") == 0) fork = argv[i + 1];
    }
    const char* real = std::getenv("REAL_DATA");
    fs::path data = real ? real : "/srv/benchd/res/real";
    if (fork != "fulu") {
        std::fprintf(stderr, "unknown fork %s\n", fork.c_str());
        return 2;
    }
    Session s;
    s.thread();
    fs::path dir = data / fork;
    fs::path min = dir / "minimal";
    {
        namespace m = mainnet;
        namespace n = minimal;
        one<m::FuluBeaconState>(s, "FuluState", dir, "state", own<m::FuluBeaconState>);
        one<m::ElectraSignedBeaconBlock>(s, "FuluBlock", dir, "block", message<m::ElectraSignedBeaconBlock>);
        set<m::ElectraSignedBeaconBlock>(s, "FuluBlocks", dir, "blocks", message<m::ElectraSignedBeaconBlock>);
        one<n::FuluBeaconState>(s, "FuluMinState", min, "state", own<n::FuluBeaconState>);
        one<n::ElectraSignedBeaconBlock>(s, "FuluMinBlock", min, "block", message<n::ElectraSignedBeaconBlock>,
                                         kBitlistAtLimit);
    }
    return 0;
}
