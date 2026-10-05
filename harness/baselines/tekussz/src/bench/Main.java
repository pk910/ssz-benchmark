package bench;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.function.Function;
import java.util.stream.Stream;
import org.apache.tuweni.bytes.Bytes;
import org.apache.tuweni.bytes.Bytes32;
import tech.pegasys.teku.infrastructure.ssz.SszContainer;
import tech.pegasys.teku.infrastructure.ssz.schema.SszContainerSchema;

/**
 * The adapter of Teku's SSZ library for benchwrap: the harness objects deserialized with the
 * generated schemas (GenFulu.java, GenGloas.java, one class per preset), every operation measured
 * with the kit's policy, the figures sent over the protocol (harness/benchwrap).
 *
 * <p>Operations: Unmarshal (sszDeserialize into the tree-backed container; Teku builds the backing
 * tree eagerly, so this is the decode), SizeSSZ (the schema's getSszSize of the tree), Marshal
 * (sszSerialize to a new Bytes) and HashTreeRoot. Teku caches every hash in its tree nodes, so
 * hashTreeRoot of an object once hashed is a lookup: HashTreeRoot hashes a freshly deserialized
 * object per iteration and the deserialization is part of the measured operation. Memory is the
 * bytes allocated by the measuring thread (ThreadMXBean); the runtime does not count allocations.
 *
 * <p>Objects of a fork: the state, the block, the block set, and (Gloas) the envelope, plus the
 * minimal-preset state and block. Roots: a state's own, a signed block's or envelope's Message.
 */
public final class Main {
  static final String ENGINE = "Teku";
  static final String[] OPS = {"Unmarshal", "SizeSSZ", "Marshal", "HashTreeRoot"};

  /** The root of a signed object is its Message's, the first field. */
  static final Function<SszContainer, Bytes32> SELF = SszContainer::hashTreeRoot;

  static final Function<SszContainer, Bytes32> MESSAGE = c -> c.get(0).hashTreeRoot();

  public static void main(final String[] args) throws IOException {
    final String fork = args.length > 0 ? args[0] : "fulu";
    final String data = System.getenv("REAL_DATA");
    final Path dir = Path.of(data == null || data.isEmpty() ? "/srv/benchd/res/real" : data, fork);
    final Path min = dir.resolve("minimal");
    final Protocol p = new Protocol();
    p.thread();
    switch (fork) {
      case "fulu":
        one(p, "FuluState", dir, "state", GenFuluMainnet.FuluBeaconState, SELF, true);
        one(p, "FuluBlock", dir, "block", GenFuluMainnet.ElectraSignedBeaconBlock, MESSAGE, false);
        set(p, "FuluBlocks", dir, "blocks", GenFuluMainnet.ElectraSignedBeaconBlock, MESSAGE);
        one(p, "FuluMinState", min, "state", GenFuluMinimal.FuluBeaconState, SELF, true);
        one(p, "FuluMinBlock", min, "block", GenFuluMinimal.ElectraSignedBeaconBlock, MESSAGE, false);
        break;
      case "gloas":
        one(p, "GloasState", dir, "state", GenGloasMainnet.GloasBeaconState, SELF, true);
        one(p, "GloasBlock", dir, "block", GenGloasMainnet.GloasSignedBeaconBlock, MESSAGE, false);
        set(p, "GloasBlocks", dir, "blocks", GenGloasMainnet.GloasSignedBeaconBlock, MESSAGE);
        one(
            p,
            "GloasEnvelope",
            dir,
            "envelope",
            GenGloasMainnet.GloasSignedExecutionPayloadEnvelope,
            MESSAGE,
            false);
        one(p, "GloasMinState", min, "state", GenGloasMinimal.GloasBeaconState, SELF, true);
        one(p, "GloasMinBlock", min, "block", GenGloasMinimal.GloasSignedBeaconBlock, MESSAGE, false);
        break;
      default:
        System.err.println("unknown fork " + fork);
        System.exit(2);
    }
  }

  /** The leaves of an object the pattern selects. */
  static List<Protocol.Leaf> leaves(final Protocol p, final String object) {
    final List<Protocol.Leaf> out = new ArrayList<>();
    for (String op : OPS) {
      final Protocol.Leaf l = new Protocol.Leaf(ENGINE, object, op);
      if (p.matches(l)) {
        out.add(l);
      }
    }
    return out;
  }

  /** A payload: the bytes of one or more objects with their roots. */
  record Item(Bytes data, Bytes32 root) {}

  static Bytes32 readRoot(final Path path) throws IOException {
    return Bytes32.fromHexString(Files.readString(path).trim());
  }

  static List<Item> loadOne(final Path dir, final String name) throws IOException {
    final Bytes data = Bytes.wrap(Files.readAllBytes(dir.resolve(name + ".ssz")));
    return List.of(new Item(data, readRoot(dir.resolve(name + ".root"))));
  }

  static List<Item> loadSet(final Path dir, final String name) throws IOException {
    final List<Path> files;
    try (Stream<Path> s = Files.list(dir.resolve(name))) {
      files = s.filter(f -> f.toString().endsWith(".ssz")).sorted().toList();
    }
    final List<Item> items = new ArrayList<>();
    for (Path f : files) {
      final String stem = f.toString().substring(0, f.toString().length() - 4);
      items.add(new Item(Bytes.wrap(Files.readAllBytes(f)), readRoot(Path.of(stem + ".root"))));
    }
    if (items.isEmpty()) {
      throw new IOException(dir.resolve(name) + ": no files");
    }
    return items;
  }

  static void one(
      final Protocol p,
      final String object,
      final Path dir,
      final String file,
      final SszContainerSchema<SszContainer> schema,
      final Function<SszContainer, Bytes32> rootOf,
      final boolean state)
      throws IOException {
    final List<Protocol.Leaf> ls = leaves(p, object);
    if (ls.isEmpty()) {
      return;
    }
    bench(p, ls, loadOne(dir, file), schema, rootOf, state);
  }

  static void set(
      final Protocol p,
      final String object,
      final Path dir,
      final String name,
      final SszContainerSchema<SszContainer> schema,
      final Function<SszContainer, Bytes32> rootOf)
      throws IOException {
    final List<Protocol.Leaf> ls = leaves(p, object);
    if (ls.isEmpty()) {
      return;
    }
    bench(p, ls, loadSet(dir, name), schema, rootOf, false);
  }

  /**
   * Deserializes every item of the payload and checks it as the kit does: the re-serialized bytes
   * equal the input, the root equals the stored one. The message names the first difference.
   * Nothing is kept: a hashed Teku object carries a root in every branch node, and a state so
   * enlarged next to the one an iteration builds does not fit the heap.
   */
  static void verify(
      final List<Item> items,
      final SszContainerSchema<SszContainer> schema,
      final Function<SszContainer, Bytes32> rootOf) {
    for (Item it : items) {
      final SszContainer v;
      try {
        v = schema.sszDeserialize(it.data());
      } catch (RuntimeException e) {
        throw new IllegalStateException("decode: " + e, e);
      }
      if (!v.sszSerialize().equals(it.data())) {
        throw new IllegalStateException("decoded value does not encode back to the input");
      }
      final Bytes32 got = rootOf.apply(v);
      if (!got.equals(it.root())) {
        throw new IllegalStateException("root mismatch: got " + got + " want " + it.root());
      }
    }
  }

  /** Measures the operations of an object; one iteration of a set runs the operation on every item. */
  static void bench(
      final Protocol p,
      final List<Protocol.Leaf> ls,
      final List<Item> items,
      final SszContainerSchema<SszContainer> schema,
      final Function<SszContainer, Bytes32> rootOf,
      final boolean state) {
    try {
      verify(items, schema, rootOf);
    } catch (IllegalStateException e) {
      for (Protocol.Leaf l : ls) {
        p.fail(l, e.getMessage());
      }
      return;
    }
    final int n = items.size();
    final Bytes[] data = new Bytes[n];
    for (int i = 0; i < n; i++) {
      data[i] = items.get(i).data();
    }
    for (Protocol.Leaf l : ls) {
      switch (l.op) {
        case "Unmarshal":
          p.run(
              l,
              state,
              () -> {
                final SszContainer[] out = new SszContainer[n];
                for (int i = 0; i < n; i++) {
                  out[i] = schema.sszDeserialize(data[i]);
                }
                return out;
              });
          break;
        case "SizeSSZ":
          sizeSSZ(p, l, state, decode(schema, data), schema);
          break;
        case "Marshal":
          marshal(p, l, state, decode(schema, data));
          break;
        case "HashTreeRoot":
          // Teku caches the roots in the tree: a fresh deserialization per iteration gives an
          // object without a cache, and is part of the measured operation.
          p.run(
              l,
              state,
              () -> {
                final Bytes32[] out = new Bytes32[n];
                for (int i = 0; i < n; i++) {
                  out[i] = rootOf.apply(schema.sszDeserialize(data[i]));
                }
                return out;
              });
          break;
        default:
          p.skip(l);
      }
    }
  }

  /** The decoded, unhashed objects an operation works on; they live for that leaf only. */
  static SszContainer[] decode(final SszContainerSchema<SszContainer> schema, final Bytes[] data) {
    final SszContainer[] out = new SszContainer[data.length];
    for (int i = 0; i < data.length; i++) {
      out[i] = schema.sszDeserialize(data[i]);
    }
    return out;
  }

  /** The size is a walk of the tree; the sum goes to a sink so that nothing is kept. */
  static void sizeSSZ(
      final Protocol p,
      final Protocol.Leaf l,
      final boolean state,
      final SszContainer[] values,
      final SszContainerSchema<SszContainer> schema) {
    p.run(
        l,
        state,
        () -> {
          long sum = 0;
          for (SszContainer v : values) {
            sum += schema.getSszSize(v.getBackingNode());
          }
          sizeSink = sum;
          return null;
        });
  }

  static void marshal(
      final Protocol p, final Protocol.Leaf l, final boolean state, final SszContainer[] values) {
    p.run(
        l,
        state,
        () -> {
          final Bytes[] out = new Bytes[values.length];
          for (int i = 0; i < values.length; i++) {
            out[i] = values[i].sszSerialize();
          }
          return out;
        });
  }

  static volatile long sizeSink;
}
