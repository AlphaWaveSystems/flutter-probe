"""Counts, for each widget in docs/compat/catalog.json, how many packages of a source tree construct it.

usage: python3 -I widget_usage.py <catalog.json> <source root> <out.json>
A package is the nearest folder with a pubspec.yaml. Files under test/, integration_test/ and generated files (*.g.dart, *.freezed.dart) are skipped.
A use is a constructor call: `Name(` or `Name.named(` not preceded by a letter, digit, underscore or dot.
"""
import json
import os
import re
import sys

catalog, root, out = sys.argv[1:4]
widgets = [w for w in json.load(open(catalog))['widgets'] if not w.get('abstract')]
names = {w['name']: w for w in widgets}
pat = re.compile(r'(?<![\w.$])(' + '|'.join(sorted(map(re.escape, names), key=len, reverse=True)) + r')(?:\.\w+)?\s*\(')

def package_of(path):
    d = os.path.dirname(path)
    while d.startswith(root):
        if os.path.exists(os.path.join(d, 'pubspec.yaml')):
            return os.path.relpath(d, root)
        d = os.path.dirname(d)
    return None

apps, calls = {}, {}
packages = set()
for dirpath, dirnames, filenames in os.walk(root):
    dirnames[:] = [d for d in dirnames if d not in ('.git', '.dart_tool', 'build', 'test', 'integration_test', 'test_driver')]
    for f in filenames:
        if not f.endswith('.dart') or f.endswith('.g.dart') or f.endswith('.freezed.dart'):
            continue
        p = os.path.join(dirpath, f)
        pkg = package_of(p)
        if pkg is None:
            continue
        packages.add(pkg)
        src = open(p, encoding='utf-8', errors='replace').read()
        src = re.sub(r'//[^\n]*', '', src)
        for m in pat.finditer(src):
            n = m.group(1)
            apps.setdefault(n, set()).add(pkg)
            calls[n] = calls.get(n, 0) + 1

rows = sorted(({'name': n, 'library': names[n]['library'], 'packages': len(apps[n]), 'calls': calls[n]} for n in apps),
              key=lambda r: (-r['packages'], -r['calls'], r['name']))
json.dump({'packages': len(packages), 'rows': rows}, open(out, 'w'), indent=1)
print('packages with Dart code:', len(packages), ' widgets used:', len(rows))
for r in rows[:70]:
    print('%-28s %-10s %3d packages %5d calls' % (r['name'], r['library'], r['packages'], r['calls']))
