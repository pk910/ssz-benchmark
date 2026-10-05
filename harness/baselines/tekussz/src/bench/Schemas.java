package bench;

import java.util.List;
import tech.pegasys.teku.infrastructure.ssz.SszContainer;
import tech.pegasys.teku.infrastructure.ssz.impl.SszContainerImpl;
import tech.pegasys.teku.infrastructure.ssz.schema.ProgressiveSchemaUtils;
import tech.pegasys.teku.infrastructure.ssz.schema.SszContainerSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.SszListSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.SszPrimitiveSchemas;
import tech.pegasys.teku.infrastructure.ssz.schema.SszProgressiveBitlistSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.SszProgressiveByteListSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.SszProgressiveListSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.SszProgressiveUInt64ListSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.SszSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.SszVectorSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.collections.SszBitlistSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.collections.SszBitvectorSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.collections.SszByteVectorSchema;
import tech.pegasys.teku.infrastructure.ssz.schema.impl.AbstractSszContainerSchema.NamedSchema;

/**
 * The vocabulary of the generated schemas (Gen*.java): one helper per SSZ shape, so that a
 * field reads as its shape and the generated code carries no Teku generics. Every container is a
 * generic {@link SszContainerImpl} over the tree Teku builds on deserialization.
 */
final class Schemas {
  private Schemas() {}

  static final SszSchema<?> U8 = SszPrimitiveSchemas.UINT8_SCHEMA;
  static final SszSchema<?> U64 = SszPrimitiveSchemas.UINT64_SCHEMA;
  static final SszSchema<?> U256 = SszPrimitiveSchemas.UINT256_SCHEMA;
  static final SszSchema<?> BOOL = SszPrimitiveSchemas.BOOLEAN_SCHEMA;

  /** A byte vector; the 32 and 4 byte ones are Teku's primitive schemas, as in its own types. */
  static SszSchema<?> bytes(final int n) {
    if (n == 32) {
      return SszPrimitiveSchemas.BYTES32_SCHEMA;
    }
    if (n == 4) {
      return SszPrimitiveSchemas.BYTES4_SCHEMA;
    }
    return SszByteVectorSchema.create(n);
  }

  static SszSchema<?> bitvector(final long bits) {
    return SszBitvectorSchema.create(bits);
  }

  static SszSchema<?> bitlist(final long maxBits) {
    return SszBitlistSchema.create(maxBits);
  }

  static SszSchema<?> pbitlist() {
    return new SszProgressiveBitlistSchema();
  }

  static SszSchema<?> vector(final SszSchema<?> element, final long length) {
    return SszVectorSchema.create(element, length);
  }

  /** A bounded list; Teku picks the packed primitive list for a primitive element. */
  static SszSchema<?> list(final SszSchema<?> element, final long maxLength) {
    return SszListSchema.create(element, maxLength);
  }

  /** A progressive list (EIP-7916); the primitive elements have their own packed schemas. */
  static SszSchema<?> plist(final SszSchema<?> element) {
    if (element == SszPrimitiveSchemas.UINT64_SCHEMA) {
      return SszProgressiveUInt64ListSchema.create();
    }
    if (element == SszPrimitiveSchemas.UINT8_SCHEMA) {
      return new SszProgressiveByteListSchema<>(SszPrimitiveSchemas.UINT8_SCHEMA);
    }
    return SszProgressiveListSchema.create(element);
  }

  static NamedSchema<?> f(final String name, final SszSchema<?> schema) {
    return NamedSchema.of(name, schema);
  }

  static SszContainerSchema<SszContainer> container(
      final String name, final NamedSchema<?>... fields) {
    return SszContainerSchema.create(name, List.of(fields), SszContainerImpl::new);
  }

  /** A progressive container (EIP-7495) with every declared field active. */
  static SszContainerSchema<SszContainer> progressive(
      final String name, final NamedSchema<?>... fields) {
    return SszContainerSchema.createProgressive(
        name, ProgressiveSchemaUtils.allActive(fields.length), List.of(fields));
  }
}
