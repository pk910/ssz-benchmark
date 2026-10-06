package bench;

import java.io.IOException;
import java.io.OutputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.function.Function;
import java.util.stream.Stream;
import org.apache.tuweni.bytes.Bytes;
import org.apache.tuweni.bytes.Bytes32;
import tech.pegasys.teku.infrastructure.ssz.SszContainer;
import tech.pegasys.teku.infrastructure.ssz.SszData;
import tech.pegasys.teku.infrastructure.ssz.schema.SszContainerSchema;
import tech.pegasys.teku.infrastructure.ssz.sos.SszOutputStreamWriter;
import tech.pegasys.teku.infrastructure.ssz.sos.SszWriter;

/**
 * The adapter of Teku's SSZ library for benchwrap: the harness objects deserialized with the
 * generated schemas (GenFulu.java, GenGloas.java, one class per preset), every operation measured
 * with the kit's policy, the figures sent over the protocol (harness/benchwrap).
 *
 * <p>Operations: Unmarshal (sszDeserialize: Teku builds the whole backing tree eagerly, a leaf per
 * 32-byte chunk and every branch above it, so this is the full decode; only the typed views over
 * the tree are created on access), SizeSSZ (the schema's getSszSize of the tree: read from the
 * length nodes, so microseconds for a state), Marshal (sszSerialize into a new byte array sized by
 * getSszSize), MarshalTo (sszSerialize into a caller's SszWriter over one byte array kept across
 * iterations), MarshalWriter (sszSerialize through the library's SszOutputStreamWriter, its
 * serialization into any OutputStream, here one over a kept byte array) and HashTreeRoot. Teku caches every root in its tree nodes, so hashTreeRoot of an
 * object once hashed is a lookup: HashTreeRoot hashes a freshly deserialized object per iteration,
 * prepared outside the timed window, so that only the hashing counts. Memory is the bytes
 * allocated by the measuring thread (ThreadMXBean); the runtime does not count allocations.
 *
 * <p>Objects of a fork: the state, the block, the block set, and (Gloas) the envelope, plus the
 * minimal-preset state and block. Roots: a state's own, a signed block's or envelope's Message.
 */
public final class Main {
  static final String ENGINE = "Teku";
  static final String[] OPS = {
    "Unmarshal", "SizeSSZ", "Marshal", "MarshalTo", "MarshalWriter", "HashTreeRoot"
  };

  /** The part of an object whose root is stored: a state itself, a signed object's Message. */
  static final Function<SszContainer, SszData> SELF = c -> c;

  static final Function<SszContainer, SszData> MESSAGE = c -> c.get(0);

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
      final Function<SszContainer, SszData> target,
      final boolean state)
      throws IOException {
    final List<Protocol.Leaf> ls = leaves(p, object);
    if (ls.isEmpty()) {
      return;
    }
    bench(p, ls, loadOne(dir, file), schema, target, state);
  }

  static void set(
      final Protocol p,
      final String object,
      final Path dir,
      final String name,
      final SszContainerSchema<SszContainer> schema,
      final Function<SszContainer, SszData> target)
      throws IOException {
    final List<Protocol.Leaf> ls = leaves(p, object);
    if (ls.isEmpty()) {
      return;
    }
    bench(p, ls, loadSet(dir, name), schema, target, false);
  }

  /** Measures the operations of an object; one iteration of a set runs the operation on every item. */
  static void bench(
      final Protocol p,
      final List<Protocol.Leaf> ls,
      final List<Item> items,
      final SszContainerSchema<SszContainer> schema,
      final Function<SszContainer, SszData> target,
      final boolean state) {
    // Nothing is verified up front: every operation checks its own result after its loop (a
    // decode by re-encoding it, an encoding against the input, roots against the stored ones),
    // and a decode, encode and hash of the object beforehand would repeat that at a quarter of
    // a minute per state.
    final int n = items.size();
    final Bytes[] data = new Bytes[n];
    long total = 0;
    int longest = 0;
    for (int i = 0; i < n; i++) {
      data[i] = items.get(i).data();
      total += data[i].size();
      longest = Math.max(longest, data[i].size());
    }
    for (Protocol.Leaf l : ls) {
      try {
        runLeaf(p, l, items, data, schema, target, state, total, longest);
      } catch (RuntimeException e) {
        // The library refused the operation (an object it cannot decode, say): this leaf fails,
        // the others go on. A window the loop may have opened is closed first.
        p.abort(l, String.valueOf(e));
      }
    }
  }

  /** Measures one operation of an object. */
  static void runLeaf(
      final Protocol p,
      final Protocol.Leaf l,
      final List<Item> items,
      final Bytes[] data,
      final SszContainerSchema<SszContainer> schema,
      final Function<SszContainer, SszData> target,
      final boolean state,
      final long total,
      final int longest) {
    {
      switch (l.op) {
        case "Unmarshal":
          p.run(l, state, () -> decode(schema, data));
          break;
        case "SizeSSZ":
          sizeSSZ(p, l, state, decode(schema, data), schema, total);
          break;
        case "Marshal":
          marshal(p, l, state, decode(schema, data));
          break;
        case "MarshalTo":
          marshalTo(p, l, state, decode(schema, data), data, longest);
          break;
        case "MarshalWriter":
          marshalWriter(p, l, state, decode(schema, data), data, longest);
          break;
        case "HashTreeRoot":
          // Teku caches the roots in the tree: every iteration hashes a freshly deserialized
          // object, prepared untimed together with the choice of the part to hash, so that the
          // measured work is the hashing alone.
          p.run(
              l,
              state,
              () -> targets(decode(schema, data), target),
              in -> {
                final SszData[] objs = (SszData[]) in;
                final Bytes32[] out = new Bytes32[objs.length];
                for (int i = 0; i < objs.length; i++) {
                  out[i] = objs[i].hashTreeRoot();
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

  /** The parts of the decoded objects whose roots are measured (a view over the tree, unhashed). */
  static SszData[] targets(
      final SszContainer[] values, final Function<SszContainer, SszData> target) {
    final SszData[] out = new SszData[values.length];
    for (int i = 0; i < values.length; i++) {
      out[i] = target.apply(values[i]);
    }
    return out;
  }

  /**
   * The size is read from the tree's length nodes (a walk of the variable-size fields); it is
   * checked once against the input length outside the loop, and the sum goes to a sink so that
   * nothing is kept.
   */
  static void sizeSSZ(
      final Protocol p,
      final Protocol.Leaf l,
      final boolean state,
      final SszContainer[] values,
      final SszContainerSchema<SszContainer> schema,
      final long total) {
    long size = 0;
    for (SszContainer v : values) {
      size += schema.getSszSize(v.getBackingNode());
    }
    if (size != total) {
      p.fail(l, "size " + size + " want " + total);
      return;
    }
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

  /** An SszWriter over one byte array, rewound for every object written into it. */
  static final class ArrayWriter implements SszWriter {
    final byte[] buf;
    int pos;

    ArrayWriter(final int capacity) {
      buf = new byte[capacity];
    }

    @Override
    public void write(final byte[] bytes, final int offset, final int length) {
      System.arraycopy(bytes, offset, buf, pos, length);
      pos += length;
    }
  }

  /**
   * Encodes into a buffer kept across iterations (the kit's MarshalTo: the buffer of the longest
   * item, rewound per item); the output is checked once against the input outside the loop.
   */
  static void marshalTo(
      final Protocol p,
      final Protocol.Leaf l,
      final boolean state,
      final SszContainer[] values,
      final Bytes[] data,
      final int longest) {
    final ArrayWriter w = new ArrayWriter(longest);
    for (int i = 0; i < values.length; i++) {
      w.pos = 0;
      values[i].sszSerialize(w);
      final byte[] want = data[i].toArrayUnsafe();
      if (!Arrays.equals(w.buf, 0, w.pos, want, 0, want.length)) {
        p.fail(l, "item " + i + " marshalTo output differs");
        return;
      }
    }
    p.run(
        l,
        state,
        () -> {
          for (SszContainer v : values) {
            w.pos = 0;
            v.sszSerialize(w);
          }
          return null;
        });
  }

  /** An OutputStream over one byte array, rewound for every object written into it. */
  static final class ArrayOutputStream extends OutputStream {
    final byte[] buf;
    int pos;

    ArrayOutputStream(final int capacity) {
      buf = new byte[capacity];
    }

    @Override
    public void write(final int b) {
      buf[pos++] = (byte) b;
    }

    @Override
    public void write(final byte[] bytes, final int offset, final int length) {
      System.arraycopy(bytes, offset, buf, pos, length);
      pos += length;
    }
  }

  /**
   * Encodes through the library's stream writer ({@link SszOutputStreamWriter}, the serialization
   * into any OutputStream) into one stream kept across iterations and rewound per item: the kit's
   * MarshalWriter. The output is checked once against the input outside the loop.
   */
  static void marshalWriter(
      final Protocol p,
      final Protocol.Leaf l,
      final boolean state,
      final SszContainer[] values,
      final Bytes[] data,
      final int longest) {
    final ArrayOutputStream out = new ArrayOutputStream(longest);
    final SszWriter w = new SszOutputStreamWriter(out);
    for (int i = 0; i < values.length; i++) {
      out.pos = 0;
      values[i].sszSerialize(w);
      final byte[] want = data[i].toArrayUnsafe();
      if (!Arrays.equals(out.buf, 0, out.pos, want, 0, want.length)) {
        p.fail(l, "item " + i + " marshalWriter output differs");
        return;
      }
    }
    p.run(
        l,
        state,
        () -> {
          for (SszContainer v : values) {
            out.pos = 0;
            v.sszSerialize(w);
          }
          return null;
        });
  }

  static volatile long sizeSink;
}
