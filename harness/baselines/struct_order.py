"""Prints the struct types of a Go file with every type after the types it
refers to."""
import re, sys
src = open(sys.argv[1]).read()
structs, cur = {}, None
for line in src.split('\n'):
    m = re.match(r'^type (\w+) struct', line)
    if m:
        cur = m.group(1); structs[cur] = set(); continue
    if cur and line.startswith('}'):
        cur = None; continue
    if cur:
        fm = re.match(r'^\t\w+\s+(\S+)', line)
        if fm:
            structs[cur].update(re.findall(r'\b([A-Z]\w*)\b', fm.group(1)))
order, done = [], set()
def visit(n):
    if n in done or n not in structs:
        return
    for d in sorted(structs[n]):
        if d != n:
            visit(d)
    done.add(n); order.append(n)
for n in sorted(structs):
    visit(n)
print('\n'.join(order))
