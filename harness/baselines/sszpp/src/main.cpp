// The adapter of sszpp for benchwrap: the harness objects decoded into
// the generated C++ types (gen_fulu.hpp, one namespace per preset), every
// operation measured with the kit's policy, the figures sent over the
// protocol (harness/benchwrap).
//
// Operations: Unmarshal (ssz::deserialize<T*>, the object on the heap as
// the library's own benchmark decodes a state), SizeSSZ (ssz_size),
// Marshal (ssz::serialize into a new buffer), MarshalTo (ssz::serialize
// through an iterator into a buffer kept across iterations; the library
// ORs bit fields into the destination, so the buffer is zeroed first) and
// HashTreeRoot (ssz::hash_tree_root on one thread; the library splits
// large vectors over std::async threads when asked for more). Objects of
// the fulu launcher: the state, the block, the block set, plus the
// minimal-preset state. The minimal-preset block is left out: its
// attestations carry aggregation bits at the limit (8192 bits in 1025
// bytes), and the library's bitlist deserialization refuses a bitlist
// whose byte count times eight exceeds the limit, which counts the byte of
// the length bit (lists.hpp, "byte slice larger than list limit"). The
// Gloas types are progressive containers and lists, which sszpp has no
// notion of: no gloas launcher.
//
// Memory: a counting operator new gives the bytes and allocations per
// operation. Freeing is not part of an operation: as the kit collects
// between batches with the clock stopped, the results of a batch are
// destroyed between batches with the counters paused.
#include <algorithm>
#include <atomic>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <filesystem>
#include <fstream>
#include <memory>
#include <new>
#include <stdexcept>
#include <string>
#include <vector>

#include "gen_fulu.hpp"
#include "protocol.hpp"

namespace {

std::atomic<std::uint64_t> allocBytes{0}, allocCount{0};

void* counted(std::size_t n) {
    allocBytes.fetch_add(n, std::memory_order_relaxed);
    allocCount.fetch_add(1, std::memory_order_relaxed);
    void* p = std::malloc(n ? n : 1);
    if (!p) throw std::bad_alloc();
    return p;
}

void* countedAligned(std::size_t n, std::align_val_t al) {
    allocBytes.fetch_add(n, std::memory_order_relaxed);
    allocCount.fetch_add(1, std::memory_order_relaxed);
    std::size_t a = static_cast<std::size_t>(al);
    void* p = std::aligned_alloc(a, (n + a - 1) / a * a);
    if (!p) throw std::bad_alloc();
    return p;
}

}  // namespace

void* operator new(std::size_t n) { return counted(n); }
void* operator new[](std::size_t n) { return counted(n); }
void* operator new(std::size_t n, const std::nothrow_t&) noexcept { return std::malloc(n ? n : 1); }
void* operator new[](std::size_t n, const std::nothrow_t&) noexcept { return std::malloc(n ? n : 1); }
void* operator new(std::size_t n, std::align_val_t al) { return countedAligned(n, al); }
void* operator new[](std::size_t n, std::align_val_t al) { return countedAligned(n, al); }
void operator delete(void* p) noexcept { std::free(p); }
void operator delete[](void* p) noexcept { std::free(p); }
void operator delete(void* p, std::size_t) noexcept { std::free(p); }
void operator delete[](void* p, std::size_t) noexcept { std::free(p); }
void operator delete(void* p, std::align_val_t) noexcept { std::free(p); }
void operator delete[](void* p, std::align_val_t) noexcept { std::free(p); }
void operator delete(void* p, std::size_t, std::align_val_t) noexcept { std::free(p); }
void operator delete[](void* p, std::size_t, std::align_val_t) noexcept { std::free(p); }

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

Payload loadSet(const fs::path& dir, const std::string& name) {
    std::vector<fs::path> files;
    for (const auto& e : fs::directory_iterator(dir / name)) {
        if (e.path().extension() == ".ssz") files.push_back(e.path());
    }
    std::sort(files.begin(), files.end());
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
// operation on every item.
template <class T>
void bench(Session& s, const std::vector<Leaf>& ls, const Payload& p, RootOf<T> rootOf) {
    std::vector<std::unique_ptr<T>> decoded;
    try {
        decoded = verify<T>(p, rootOf);
    } catch (const std::exception& e) {
        for (const auto& l : ls) s.fail(l, e.what());
        return;
    }
    std::size_t total = 0;
    for (const auto& [d, _] : p.items) total += d.size();
    for (const auto& l : ls) {
        if (l.op == "Unmarshal") {
            s.run(l, [&] {
                std::vector<std::unique_ptr<T>> out;
                out.reserve(p.items.size());
                for (const auto& [d, _] : p.items) out.emplace_back(ssz::deserialize<T*>(d));
                return out;
            });
        } else if (l.op == "SizeSSZ") {
            s.run(l, [&] {
                std::size_t sum = 0;
                for (const auto& v : decoded) sum += v->ssz_size();
                return sum;
            });
        } else if (l.op == "Marshal") {
            s.run(l, [&] {
                std::vector<std::vector<std::byte>> out;
                out.reserve(decoded.size());
                for (const auto& v : decoded) out.push_back(ssz::serialize(*v));
                return out;
            });
        } else if (l.op == "MarshalTo") {
            std::vector<std::byte> buf(total);
            s.run(l, [&] {
                std::memset(buf.data(), 0, buf.size());
                std::size_t off = 0;
                for (std::size_t i = 0; i < decoded.size(); i++) {
                    ssz::serialize(buf.begin() + static_cast<std::ptrdiff_t>(off), *decoded[i]);
                    off += p.items[i].first.size();
                }
                return off;
            });
        } else if (l.op == "HashTreeRoot") {
            s.run(l, [&] {
                std::vector<ssz::chunk_t> out;
                out.reserve(decoded.size());
                for (const auto& v : decoded) out.push_back(rootOf(*v));
                return out;
            });
        } else {
            s.skip(l);
        }
    }
}

template <class T>
void one(Session& s, const std::string& object, const fs::path& dir, const std::string& file, RootOf<T> rootOf) {
    auto ls = leaves(s, object);
    if (ls.empty()) return;
    bench<T>(s, ls, loadOne(dir, file), rootOf);
}

template <class T>
void set(Session& s, const std::string& object, const fs::path& dir, const std::string& name, RootOf<T> rootOf) {
    auto ls = leaves(s, object);
    if (ls.empty()) return;
    bench<T>(s, ls, loadSet(dir, name), rootOf);
}

template <class T>
ssz::chunk_t own(const T& v) {
    return ssz::hash_tree_root(v, 1);
}

template <class T>
ssz::chunk_t message(const T& v) {
    return ssz::hash_tree_root(v.Message, 1);
}

}  // namespace

int main(int argc, char** argv) {
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
        // FuluMinBlock: a full bitlist, which the library cannot decode (see above).
    }
    return 0;
}
