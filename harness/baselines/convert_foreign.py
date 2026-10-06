#!/usr/bin/env python3
"""Converts the harness type definitions (types/<fork>/types.go, dynamic-ssz
style) into the type definitions of an SSZ library of another language,
with the spec expressions evaluated for a preset, so that every library
measures exactly the harness's schema.

    convert_foreign.py <dialect> <types.go> <out> --roots A,B --preset name=spec.json [...]

Dialects: rust (ethereum_ssz, ssz_types, tree_hash) here; any other in
convert_<dialect>.py next to this file. One module per preset
is written into the output file. The dynamic-ssz annotations give the
expressions; a name the preset does not define falls back to the static
tag, as dynamic-ssz does.
"""
import json
import re
import sys

# ---- parsing the Go types -------------------------------------------------

FIELD = re.compile(r'^\t(\w+)\s+(\S+)(?:\s+`([^`]*)`)?\s*$')
BASE = re.compile(r'^type (\w+) (\[\d+\]byte|uint64|uint32|uint16|uint8|bool|\[\]byte|\[\]\w+)$')
TAG = re.compile(r'(\w[\w-]*):"([^"]*)"')


def parse(src):
    """Returns the aliases (name -> go type) and the structs (name -> list
    of (field, go type, tags dict)) in source order."""
    aliases, structs, order = {}, {}, []
    cur = None
    for line in src.split('\n'):
        m = BASE.match(line)
        if m:
            aliases[m.group(1)] = m.group(2)
            continue
        m = re.match(r'^type (\w+) struct', line)
        if m:
            cur = m.group(1)
            structs[cur] = []
            order.append(cur)
            continue
        if cur and line.startswith('}'):
            cur = None
            continue
        if cur:
            m = FIELD.match(line)
            if m:
                name, typ, tag = m.groups()
                structs[cur].append((name, typ, dict(TAG.findall(tag or ''))))
    return aliases, structs, order


def reachable(structs, aliases, roots):
    keep, stack = set(), list(roots)
    while stack:
        n = stack.pop()
        if n in keep or n not in structs:
            continue
        keep.add(n)
        for _, typ, _ in structs[n]:
            for name in re.findall(r'\b([A-Z]\w*)\b', typ):
                if name in structs:
                    stack.append(name)
                elif name in aliases:
                    inner = aliases[name]
                    for x in re.findall(r'\b([A-Z]\w*)\b', inner):
                        if x in structs:
                            stack.append(x)
    return keep


# ---- the spec expressions -------------------------------------------------

class Preset:
    def __init__(self, name, path):
        self.name = name
        self.values = {}
        if path:
            data = json.load(open(path))['data']
            for k, v in data.items():
                if isinstance(v, str) and v.isdigit():
                    self.values[k] = int(v)

    def eval(self, expr):
        """Evaluates +, *, / (rounding up, as dynamic-ssz sizes bytes) and
        parentheses over spec names; None when a name is undefined."""
        tokens = re.findall(r'\d+|[A-Z_][A-Z_0-9]*|[()+*/-]', expr)
        pos = 0

        def peek():
            return tokens[pos] if pos < len(tokens) else None

        def take():
            nonlocal pos
            pos += 1
            return tokens[pos - 1]

        def atom():
            t = take()
            if t == '(':
                v = add()
                take()
                return v
            if t.isdigit():
                return int(t)
            if t not in self.values:
                raise KeyError(t)
            return self.values[t]

        def mul():
            v = atom()
            while peek() in ('*', '/'):
                if take() == '*':
                    v *= atom()
                else:
                    d = atom()
                    v = -(-v // d)
            return v

        def add():
            v = mul()
            while peek() in ('+', '-'):
                if take() == '+':
                    v += mul()
                else:
                    v -= mul()
            return v
        try:
            return add()
        except KeyError:
            return None


def dims(tags, preset, key):
    """The sizes (key 'size') or limits (key 'max') per dimension: the
    dynamic expression evaluated for the preset, else the static tag;
    None for an unsized dimension ('?')."""
    static = [x.strip() for x in tags.get('ssz-' + key, '').split(',')] if tags.get('ssz-' + key) else []
    dynamic = [x.strip() for x in tags.get('dynssz-' + key, '').split(',')] if tags.get('dynssz-' + key) else []
    n = max(len(static), len(dynamic))
    out = []
    for i in range(n):
        v = None
        if i < len(dynamic) and dynamic[i] and dynamic[i] != '?':
            v = preset.eval(dynamic[i])
        if v is None and i < len(static) and static[i] and static[i] != '?':
            v = int(static[i])
        out.append(v)
    return out


# ---- the shape of a field ---------------------------------------------------

class Shape:
    """A field's SSZ shape: a chain of levels over a base. Each level is
    ('vector', n) / ('list', limit) / ('plist', limit or 0) and the base is
    ('uint', bits) / ('bool',) / ('bytes', n) / ('struct', name) /
    ('bitvector', bits) / ('bitlist', limit) / ('pbitlist',) / ('u256',)."""

    def __init__(self, levels, base):
        self.levels, self.base = levels, base


def shape(typ, tags, aliases, preset):
    typ = typ.lstrip('*')
    kinds = [x.strip() for x in tags.get('ssz-type', '').split(',')] if tags.get('ssz-type') else []
    sizes = dims(tags, preset, 'size')
    maxes = dims(tags, preset, 'max')
    if typ == 'uint256.Int':
        return Shape([], ('u256',))
    if typ.startswith('bitfield.Bitlist'):
        if kinds[:1] == ['progressive-bitlist']:
            return Shape([], ('pbitlist',))
        return Shape([], ('bitlist', maxes[0]))
    m = re.match(r'^bitfield\.Bitvector(\d+)$', typ)
    if m:
        bits = int(m.group(1))
        if tags.get('ssz-bitsize'):
            bits = int(tags['ssz-bitsize'])
        elif tags.get('dynssz-size'):
            expr = tags['dynssz-size']
            # A size in bytes written as <bits>/8: the bits are the
            # expression without the division.
            v = preset.eval(expr[:-2]) if expr.endswith('/8') else None
            if v is None:
                s = preset.eval(expr)
                v = s * 8 if s is not None else None
            if v is not None:
                bits = v
        return Shape([], ('bitvector', bits))
    # Resolve alias chains, inlining the Go type.
    seen = set()
    while typ in aliases and typ not in seen:
        seen.add(typ)
        typ = aliases[typ]
    levels = []
    level = 0
    while True:
        m = re.match(r'^\[(\d*)\](.*)$', typ)
        if not m:
            break
        n, rest = m.group(1), m.group(2)
        rest = rest.lstrip('*')
        kind = kinds[level] if level < len(kinds) else ''
        size = sizes[level] if level < len(sizes) else None
        limit = maxes[level] if level < len(maxes) else None
        if n:
            levels.append(('vector', int(n)))
        elif kind == 'progressive-list':
            levels.append(('plist', limit or 0))
        elif size is not None:
            levels.append(('vector', size))
        elif limit is not None:
            levels.append(('list', limit))
        else:
            raise ValueError(f'unsized level {level} of {typ} {tags}')
        # An alias in element position is inlined as well.
        seen = set()
        while rest in aliases and rest not in seen:
            seen.add(rest)
            rest = aliases[rest]
        typ = rest
        level += 1
    if typ == 'byte' or typ == 'uint8':
        base = ('uint', 8)
    elif typ in ('uint16', 'uint32', 'uint64'):
        base = ('uint', int(typ[4:]))
    elif typ == 'bool':
        base = ('bool',)
    else:
        base = ('struct', typ)
    # A trailing level of bytes with a fixed size is a byte vector: [n]byte.
    if base == ('uint', 8) and levels and levels[-1][0] == 'vector':
        n = levels.pop()[1]
        base = ('bytes', n)
    return Shape(levels, base)


# ---- Rust -------------------------------------------------------------------

def typenum(n):
    return f'U{n}'


def rust_base(base):
    kind = base[0]
    if kind == 'uint':
        return f'u{base[1]}'
    if kind == 'bool':
        return 'bool'
    if kind == 'bytes':
        return f'[u8; {base[1]}]'
    if kind == 'struct':
        return base[1]
    if kind == 'u256':
        return 'U256'
    if kind == 'bitvector':
        return f'BitVector<{typenum(base[1])}>'
    if kind == 'bitlist':
        return f'BitList<{typenum(base[1])}>'
    if kind == 'pbitlist':
        return 'ProgressiveBitList'
    raise ValueError(base)


def rust_type(sh):
    t = rust_base(sh.base)
    for kind, n in reversed(sh.levels):
        if kind == 'vector':
            t = f'FixedVector<{t}, {typenum(n)}>'
        elif kind == 'list':
            t = f'VariableList<{t}, {typenum(n)}>'
        else:
            t = f'ProgressiveVariableList<{t}, {typenum(n)}>'
    return t


def emit_rust(aliases, structs, order, keep, presets):
    out = ['// Generated by baselines/convert_foreign.py from the harness types: do not edit.',
           '#![allow(non_snake_case, dead_code, unused_imports, clippy::all)]', '']
    for preset in presets:
        out.append(f'pub mod {preset.name} {{')
        out.append('    use alloy_primitives::U256;')
        # The progressive types are imported only when a type uses them: a
        # library version without them still builds the other fork.
        imports_at = len(out)
        out.append(None)
        out.append('    use ssz_derive::{Decode, Encode};')
        out.append('    use ssz_types::typenum::*;')
        out.append('    use tree_hash_derive::TreeHash;')
        out.append('')
        for name in order:
            if name not in keep:
                continue
            fields = structs[name]
            progressive = any('ssz-index' in t for _, _, t in fields)
            out.append('    #[derive(Debug, Clone, PartialEq, Encode, Decode, TreeHash)]')
            if progressive:
                active = ', '.join('1' for _ in fields)
                out.append(f'    #[tree_hash(struct_behaviour = "progressive_container", active_fields({active}))]')
            out.append(f'    pub struct {name} {{')
            for fname, typ, tags in fields:
                out.append(f'        pub {fname}: {rust_type(shape(typ, tags, aliases, preset))},')
            out.append('    }')
            out.append('')
        body = '\n'.join(line for line in out[imports_at + 1:] if line is not None)
        out[imports_at] = '\n'.join([
            '    use ssz::{' + ', '.join(used_names(body, ['BitList', 'BitVector', 'ProgressiveBitList'])) + '};',
            '    use ssz_types::{' + ', '.join(used_names(body, ['FixedVector', 'ProgressiveVariableList', 'VariableList'])) + '};',
        ])
        out.append('}')
        out.append('')
    return '\n'.join(out)


def used_names(body, names):
    """The names among `names` that the generated body mentions (an import
    list without unused members, so that a library version lacking some of
    the types still compiles the types it has)."""
    return [n for n in names if re.search(r'\b' + n + r'\b', body)]


# ---- main -------------------------------------------------------------------

def main(argv):
    dialect, src_path, out_path = argv[1:4]
    roots, presets = [], []
    i = 4
    while i < len(argv):
        if argv[i] == '--roots':
            roots = argv[i + 1].split(',')
        elif argv[i] == '--preset':
            name, path = argv[i + 1].split('=', 1)
            presets.append(Preset(name, path))
        i += 2
    aliases, structs, order = parse(open(src_path).read())
    keep = reachable(structs, aliases, roots) if roots else set(order)
    if dialect == 'rust':
        text = emit_rust(aliases, structs, order, keep, presets)
    else:
        # Another dialect lives in convert_<dialect>.py next to this file,
        # with emit(aliases, structs, order, keep, presets) -> str, built on
        # parse, reachable, shape and Preset from here.
        import importlib
        import os
        sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
        text = importlib.import_module('convert_' + dialect).emit(aliases, structs, order, keep, presets)
    open(out_path, 'w').write(text)


if __name__ == '__main__':
    main(sys.argv)
