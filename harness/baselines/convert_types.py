import re, sys, os
# Converts the harness type definitions (dynamic-ssz style) into the
# dialect of an external SSZ library: 'fastssz' (ferranbt/prysm sszgen) or
# 'karalabe'.
tagre=re.compile(r'`([^`]*)`')
pair=re.compile(r'(\w[\w-]*):"([^"]*)"')
basere=re.compile(r'^type (\w+) (\[\d+\]byte|uint64|uint8|\[\]byte|\[\]\w+)$')
def parse_tags(tag):
    return dict(pair.findall(tag))
def emit_tags(d):
    if not d: return ''
    return '`'+' '.join(f'{k}:"{v}"' for k,v in d.items())+'`'
def convert(src, dialect):
    aliases={}
    for line in src.split('\n'):
        m=basere.match(line)
        if m: aliases[m.group(1)]=m.group(2)
    # resolve alias chains
    def resolve(t):
        seen=set()
        while t in aliases and t not in seen:
            seen.add(t); t=aliases[t]
        return t
    def inline(typ):
        # replace alias names inside a type expression, repeatedly
        for _ in range(8):
            new=re.sub(r'\b([A-Z]\w*)\b', lambda m: aliases[m.group(1)] if m.group(1) in aliases else m.group(1), typ)
            if new==typ: return typ
            typ=new
        return typ
    out=[]
    for line in src.split('\n'):
        if basere.match(line):
            continue  # aliases are inlined everywhere
        fm=re.match(r'^(\t\w+\s+)(\S+)(\s*)(`[^`]*`)?\s*$', line)
        if fm and not line.startswith('type '):
            name, typ, sp, tag = fm.group(1), fm.group(2), fm.group(3), fm.group(4) or ''
            tags=parse_tags(tag[1:-1]) if tag else {}
            # a progressive list has no bound; the external libraries need
            # one for validation only, so any bound above the payload works
            if tags.get('ssz-type','').startswith('progressive') and 'ssz-max' not in tags:
                dims=tags['ssz-type'].count('progressive')
                tags['ssz-max']=','.join(['1099511627776']*dims)
            tags={k:v for k,v in tags.items() if not k.startswith('dynssz') and k not in ('ssz-type','ssz-index','ssz-bitsize')}
            typ=inline(typ)
            if dialect in ('fastssz','ferranbt','methodical'):
                typ=typ.replace('*uint256.Int','[32]byte')
                m5=re.match(r'^bitfield\.Bitvector(\d+)$', typ)
                if m5 and dialect=='ferranbt':
                    nb=(int(m5.group(1))+7)//8; typ=f'[{nb}]byte'; tags={'ssz-size':str(nb)}
                if typ=='[32]byte' and 'ssz-size' not in tags: tags['ssz-size']='32'
                if typ=='bitfield.Bitlist' and dialect!='methodical': tags['ssz']='bitlist'
                m6=re.match(r'^bitfield\.Bitvector(\d+)$', typ)
                if m6 and 'ssz-size' not in tags: tags['ssz-size']=str((int(m6.group(1))+7)//8)
                m4=re.match(r'^\[\]\[(\d+)\]byte$', typ)
                if m4 and 'ssz-max' in tags and 'ssz-size' not in tags: tags['ssz-size']='?,'+m4.group(1)
                if typ=='[][]byte' and 'ssz-max' in tags and 'ssz-size' not in tags: tags['ssz-size']='?,?'
            else:  # karalabe
                m2=re.match(r'^bitfield\.Bitvector(\d+)$', typ)
                if m2:
                    n=int(m2.group(1)); nb=(n+7)//8
                    if nb in (4,8,20,31,32,48,64,96,256): typ=f'[{nb}]byte'; tags={}  # a bitvector hashes and serializes as its bytes
                    elif nb==1: typ='[1]byte'; tags={'ssz-size':str(n),'ssz':'bits'}
                    else: typ=f'[{nb}]byte'; tags={'unsupported':'bitvector'}
                elif typ=='bitfield.Bitlist':
                    tags={'ssz-max':tags.get('ssz-max','')}
                elif 'ssz-size' in tags:
                    dims=tags['ssz-size'].split(',')
                    # each dimension sizes one nesting level, outer first
                    m3=re.match(r'^((?:\[\]|\[\d+\])*)(.*)$', typ)
                    levels=re.findall(r'\[\d*\]', m3.group(1))
                    for i,d in enumerate(dims):
                        if i<len(levels) and levels[i]=='[]' and d!='?':
                            levels[i]='['+d+']'
                    typ=''.join(levels)+m3.group(2)
                    if levels and levels[0]!='[]' and m3.group(2).startswith('*'):
                        typ=''.join(levels)+m3.group(2)[1:]  # a vector of containers holds values
                    tags.pop('ssz-size',None)
                tags={k:v for k,v in tags.items() if k in ('ssz-max','ssz-size','ssz')}
            line=name+typ+(' '+emit_tags(tags) if tags else '')
        out.append(line)
    res='\n'.join(out)
    if dialect in ('fastssz','ferranbt','methodical'):
        res=res.replace('\t"github.com/holiman/uint256"\n','')
    if dialect=='methodical':
        res=res.replace('"github.com/prysmaticlabs/go-bitfield"','"github.com/OffchainLabs/go-bitfield"')
    if 'bitfield.' not in res:
        res=res.replace('\t"github.com/prysmaticlabs/go-bitfield"\n','')
    if dialect=='karalabe' and 'bitfield.' not in res:
        res=res.replace('\t"github.com/prysmaticlabs/go-bitfield"\n','')
    return res
def prune(src, roots):
    structs={}; order=[]; cur=None; body={}
    for line in src.split('\n'):
        m=re.match(r'^type (\w+) struct', line)
        if m: cur=m.group(1); structs[cur]=set(); body[cur]=[line]; order.append(cur); continue
        if cur:
            body[cur].append(line)
            if line.startswith('}'): cur=None; continue
            fm=re.match(r'^\t\w+\s+(\S+)', line)
            if fm:
                for n in re.findall(r'\b([A-Z]\w*)\b', fm.group(1)): structs[cur].add(n)
    keep=set(); stack=list(roots)
    while stack:
        n=stack.pop()
        if n in keep or n not in structs: continue
        keep.add(n); stack.extend(structs[n])
    out=[]; skip=False
    for line in src.split('\n'):
        m=re.match(r'^type (\w+) struct', line)
        if m: skip=m.group(1) not in keep
        if not skip: out.append(line)
        if skip and line.startswith('}'): skip=False
    return '\n'.join(out)
def methodical_yaml(src, package):
    """The generator config of methodical-ssz: every struct, with the
    progressive containers (fields carrying ssz-index) and progressive
    collections (ssz-type) the struct tags of the harness types declare."""
    out=['package: "%s"' % package, 'types:']
    cur=None; fields=[]
    def flush():
        if cur is None: return
        out.append('  - name: "%s"' % cur)
        idx=[int(t['ssz-index']) for _,t in fields if 'ssz-index' in t]
        if idx:
            inactive=[i for i in range(max(idx)+1) if i not in idx]
            out.append('    progressive: {}' if not inactive else '    progressive:\n      inactive_indices: %s' % inactive)
        prog=[]
        for name,t in fields:
            st=t.get('ssz-type','')
            if not st.startswith('progressive'): continue
            dims=st.split(',')
            if dims[0]=='progressive-bitlist': prog.append('      %s: {type: "ProgressiveBitlist"}' % name)
            elif len(dims)>1: prog.append('      %s: {type: "ProgressiveList", element: {type: "ProgressiveByteList"}}' % name)
            else: prog.append('      %s: {type: "ProgressiveList"}' % name)
        if prog:
            out.append('    fields:'); out.extend(prog)
    for line in src.split('\n'):
        m=re.match(r'^type (\w+) struct', line)
        if m:
            flush(); cur=m.group(1); fields=[]; continue
        if cur and line.startswith('}'):
            flush(); cur=None; continue
        if cur:
            fm=re.match(r'^\t(\w+)\s+\S+\s*(`[^`]*`)?', line)
            if fm: fields.append((fm.group(1), parse_tags(fm.group(2)[1:-1]) if fm.group(2) else {}))
    return '\n'.join(out)+'\n'
src=open(sys.argv[1]).read()
if len(sys.argv)>4: src=prune(src, sys.argv[4:])
if os.environ.get('YAML_OUT'):
    open(os.environ['YAML_OUT'],'w').write(methodical_yaml(src, os.environ['YAML_PKG']))
open(sys.argv[3],'w').write(convert(src, sys.argv[2]))
