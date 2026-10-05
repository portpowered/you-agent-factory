"""Generate build-only overlays for the experimental consolidated Go unit lane.

No repository source or production export is changed. Native Go registers the
outer tests; unsupported TestMain, examples, fuzzing and known graph conflicts
stay in native package binaries. Requires Python 3 and the repository Go version.
"""
import hashlib
import json
import pathlib
import re
import sys
ROOT=pathlib.Path.cwd()
MODULE='github.com/portpowered/infinite-you'
def objects(path):
    source = path.read_text(encoding='utf-8-sig')
    decoder = json.JSONDecoder()
    pos = 0
    while pos < len(source):
        while pos < len(source) and source[pos].isspace(): pos += 1
        if pos == len(source): break
        value, pos = decoder.raw_decode(source, pos)
        yield value

def section(source, name):
    match = re.search(r'var ' + name + r' = \[\]testing\.\w+\{(.*?)\n\}', source, re.S)
    if not match: raise ValueError('missing section ' + name)
    return match[1]

STRING = r'"(?:\\.|[^"\\])*"'
PAIR = re.compile(r'\{\s*(' + STRING + r')\s*,\s*(_(?:x)?test)\.(\w+)\s*\},')
EXAMPLE = re.compile(r'\{\s*(' + STRING + r')\s*,\s*(_(?:x)?test)\.(\w+)\s*,\s*(' + STRING + r')\s*,\s*(true|false)\s*\},')

def registrations(source, name, pattern=PAIR):
    body = section(source, name)
    # A toolchain-format change must stop generation, never silently omit a
    # test or benchmark because the registration parser no longer recognizes it.
    if pattern.sub('', body).strip():
        raise ValueError('unrecognized Go test registration in ' + name)
    return pattern.findall(body)

dest=pathlib.Path(sys.argv[2]);dest.mkdir(parents=True,exist_ok=True)
(dest/'sources').mkdir(exist_ok=True)
entries=list(objects(pathlib.Path(sys.argv[1])))
bases={x['ImportPath']:x for x in entries if not x.get('ForTest') and not x['ImportPath'].endswith('.test')}
mains={x['ImportPath'][:-5]:x for x in entries if x['ImportPath'].endswith('.test') and x.get('Name')=='main'}
excluded_path=dest/'excluded.json'
excluded={
 MODULE+'/pkg/services/recordings/internal/projections': 'combined private test imports form a cycle through Factory Runtime and Recordings HTTP',
 MODULE+'/pkg/services/factory_definitions/internal/services/authoring_layout/authoredlayout': 'empty-YAML diagnostic assertion depends on original temporary-directory naming',
}
groups,overlay,bridge_groups,children=[],{},{},{}

def write_if_changed(path, data):
    if isinstance(data, str):
        data = data.encode('utf-8')
    try:
        if path.read_bytes() == data:
            return
    except FileNotFoundError:
        pass
    path.write_bytes(data)

def add_source(virtual,source):
    replacement=dest/'sources'/(hashlib.sha256(str(virtual).encode()).hexdigest()+'.go')
    write_if_changed(replacement,source)
    overlay[str(virtual)]=str(replacement)

for index,(pkg,main) in enumerate(sorted(mains.items())):
    base=bases[pkg];source=pathlib.Path(main['GoFiles'][0]).read_text()
    test_entries=registrations(source,'tests')
    bench_entries=registrations(source,'benchmarks')
    example_entries=registrations(source,'examples',EXAMPLE)
    fuzz_entries=registrations(source,'fuzzTargets')
    leak_check=False
    if re.search(r'\b_(?:x)?test\.TestMain\(m\)',source):
        bodies=[]
        for filename in base.get('TestGoFiles',[])+base.get('XTestGoFiles',[]):
            content=(pathlib.Path(base['Dir'])/filename).read_text()
            bodies.extend(re.findall(r'func TestMain\(m \*testing.M\)\s*\{([^}]+)\}',content))
        leak_check=len(bodies)==1 and bodies[0].strip()=='goleak.VerifyTestMain(m)' and not example_entries
        if leak_check:excluded.pop(pkg,None)
        else:excluded[pkg]='custom TestMain'

    if fuzz_entries:excluded[pkg]='fuzz target: retain native seed execution'
    if example_entries:excluded[pkg]='examples: preserve native example registration'
    if pkg in excluded:continue
    group=f'TestPackage{index:04d}'
    aliases={'_test':f'p{index}','_xtest':f'x{index}'}
    used={alias for _,alias,_ in test_entries+bench_entries}|{x[1] for x in example_entries}
    imports=[]
    for kind,filenames in [('TestGoFiles',base.get('TestGoFiles',[])),('XTestGoFiles',base.get('XTestGoFiles',[]))]:
        for filename in filenames:
            original=pathlib.Path(base['Dir'])/filename
            virtual=(pathlib.Path(base['Dir'])/('monolith_'+filename.replace('_test.go','.go'))) if kind=='TestGoFiles' else (pathlib.Path(base['Dir'])/'monolithcases'/filename.replace('_test.go','.go'))
            replacement=dest/'sources'/(hashlib.sha256(str(virtual).encode()).hexdigest()+'.go')
            write_if_changed(replacement, ('//line '+original.as_posix()+':1\n').encode()+original.read_bytes())
            overlay[str(virtual)]=str(replacement)
        if filenames:
            alias='_test' if kind=='TestGoFiles' else '_xtest'
            import_path=pkg if kind=='TestGoFiles' else pkg+'/monolithcases'
            imports.append((aliases[alias] if alias in used else '_')+' '+json.dumps(import_path))
        if kind=='XTestGoFiles':
            for embed in base.get('XTestEmbedFiles',[]):
                overlay[str(pathlib.Path(base['Dir'])/'monolithcases'/embed)]=str(pathlib.Path(base['Dir'])/embed)
    tests=',\n'.join('{'+name+', '+aliases[alias]+'.'+function+'}' for name,alias,function in test_entries)
    benches=',\n'.join('{'+name+', '+aliases[alias]+'.'+function+'}' for name,alias,function in bench_entries)
    examples=',\n'.join('{'+name+', '+aliases[alias]+'.'+function+', '+output+', '+unordered+'}' for name,alias,function,output,unordered in example_entries)
    literal='{LeakCheck: '+str(leak_check).lower()+', Package: '+json.dumps(pkg)+', Name: '+json.dumps(group)+', Dir: '+json.dumps(base['Dir'])+', Tests: []testing.InternalTest{'+tests+'}, Benchmarks: []testing.InternalBenchmark{'+benches+'}, Examples: []testing.InternalExample{'+examples+'}}'
    parts=pkg.split('/')
    chain=['/'.join(parts[:i])+'/monolithbridge' for i,piece in enumerate(parts) if piece=='internal']
    chain.append(pkg+'/monolithbridge')
    bridge_groups.setdefault(chain[-1],[]).append((imports,literal))
    for parent,child in zip(chain,chain[1:]):children.setdefault(parent,set()).add(child)
    groups.append({'package':pkg,'group':group,'directory':base['Dir'],'bridge':chain[-1],'top_level_tests':[json.loads(x[0]) for x in test_entries],'example_names':[json.loads(x[0]) for x in example_entries]})

support_path=MODULE+'/internal/monolithsupport'
add_source(ROOT/'internal/monolithsupport/group.go','package monolithsupport\nimport "testing"\ntype Group struct { LeakCheck bool; Package, Name, Dir string; Tests []testing.InternalTest; Benchmarks []testing.InternalBenchmark; Examples []testing.InternalExample }\n')
all_bridges=set(bridge_groups)|set(children)
child_bridges=set().union(*children.values()) if children else set()
roots=sorted(all_bridges-child_bridges)
for bridge in sorted(all_bridges):
    local=bridge_groups.get(bridge,[])
    imports=['s '+json.dumps(support_path)]
    if local:imports.append('"testing"')
    for imported,_ in local:imports.extend(imported)
    descendant=sorted(children.get(bridge,[]))
    imports.extend(f'b{i} '+json.dumps(child) for i,child in enumerate(descendant))
    code='package monolithbridge\nimport (\n'+'\n'.join(imports)+'\n)\nfunc Cases() []s.Group {\n cases := []s.Group{'+',\n'.join(x[1] for x in local)+'}\n'
    for i in range(len(descendant)):code+=f'cases = append(cases, b{i}.Cases()...)\n'
    code+='return cases\n}\n'
    add_source(ROOT/bridge[len(MODULE)+1:]/'cases.go',code)

add_source(ROOT/'pkg/monolithpilot/cpu_windows.go',r'''package monolithpilot
import "golang.org/x/sys/windows"
func processCPUSeconds() (float64, bool) {
 var created, exited, kernel, user windows.Filetime
 if windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user) != nil { return 0, false }
 ticks := (uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) + (uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime))
 return float64(ticks)/1e7, true
}
''')
add_source(ROOT/'pkg/monolithpilot/cpu_other.go', '//go:build !windows\n\npackage monolithpilot\nfunc processCPUSeconds() (float64, bool) { return 0, false }\n')
imports=['"encoding/json"','"fmt"','"go.uber.org/goleak"','"github.com/spf13/cobra"','"os"','"sort"','"strings"','"testing"','"time"','s '+json.dumps(support_path)]
imports.extend(f'b{i} '+json.dumps(root) for i,root in enumerate(roots))
harness='package monolithpilot\nimport (\n'+'\n'.join(imports)+'\n)\n'+r'''
var helperGroup = os.Getenv("UNIT_MONOLITH_PACKAGE")
var compact = os.Getenv("UNIT_MONOLITH_COMPACT") != "" && helperGroup == ""
func emitResult(t *testing.T, pkg, name string, started time.Time) {
 if !compact { return }
 action:="pass";if t.Failed() { action="fail" } else if t.Skipped() { action="skip" }
 data,err:=json.Marshal(struct {Action,Package,Test string;Elapsed float64}{action,pkg,name,time.Since(started).Seconds()});if err!=nil { panic(err) }
 if _,err:=os.Stdout.Write(append(append([]byte("UNIT_EVENT "),data...), '\n'));err!=nil { panic(err) }
}
func enterPackage(dir, group string) (func(), error) {
 if helperGroup == group { return func() {}, nil }
 cwd, err := os.Getwd(); if err != nil { return nil, err }
 previous, present := os.LookupEnv("UNIT_MONOLITH_PACKAGE")
 if err := os.Chdir(dir); err != nil { return nil, err }
 if err := os.Setenv("UNIT_MONOLITH_PACKAGE", group); err != nil { os.Chdir(cwd); return nil, err }
 return func() { if err := os.Chdir(cwd); err != nil { panic(err) }; if present { os.Setenv("UNIT_MONOLITH_PACKAGE", previous) } else { os.Unsetenv("UNIT_MONOLITH_PACKAGE") } }, nil
}
func TestMain(m *testing.M) {
 cobra.MousetrapHelpText = ""
 if helperGroup != "" {
  prefix, selected := "TestUnitPackages/"+helperGroup+"/", false
  for i:=1;i<len(os.Args);i++ {
   arg:=os.Args[i]; if arg=="--" { break }
   if arg=="-test.run" && i+1<len(os.Args) { os.Args[i+1]=prefix+os.Args[i+1]; selected=true; i++; continue }
   if strings.HasPrefix(arg,"-test.run=") { os.Args[i]="-test.run="+prefix+strings.TrimPrefix(arg,"-test.run="); selected=true }
  }
  if !selected { os.Args=append(os.Args[:1],append([]string{"-test.run="+prefix},os.Args[1:]...)...) }
 }
 os.Exit(m.Run())
}
func groups() []s.Group {
 groups := []s.Group{}
'''+'\n'.join(f'groups = append(groups, b{i}.Cases()...)' for i in range(len(roots)))+r'''
 sort.Slice(groups,func(i,j int) bool { if groups[i].LeakCheck != groups[j].LeakCheck { return groups[i].LeakCheck }; return groups[i].Name<groups[j].Name })
 return groups
}
func TestUnitPackages(t *testing.T) {
 for _,group := range groups() { t.Run(group.Name, func(t *testing.T) {
  started:=time.Now();t.Cleanup(func() {emitResult(t,group.Package,"",started)})
  if os.Getenv("UNIT_MONOLITH_GROUP_CPU") != "" {
   if before, ok := processCPUSeconds(); ok { t.Cleanup(func() { after,_ := processCPUSeconds(); fmt.Fprintf(os.Stderr,"UNIT_GROUP_CPU {\"package\":%q,\"cpu_seconds\":%.6f}\n",group.Package,after-before) }) }
  }
  restore,err:=enterPackage(group.Dir,group.Name);if err!=nil {t.Fatal(err)};t.Cleanup(restore)
  if group.LeakCheck { t.Cleanup(func() { if !t.Failed() { goleak.VerifyNone(t) } }) }
  for _,test:=range group.Tests {t.Run(test.Name,func(t *testing.T) {
   started:=time.Now();t.Cleanup(func() {emitResult(t,group.Package,test.Name,started)})
   test.F(t)
  })}
 }) }
}
func BenchmarkUnitPackages(b *testing.B) {
 for _,group:= range groups() { for _,bench := range group.Benchmarks { b.Run(group.Name+"/"+bench.Name, func(b *testing.B) {
  restore,err:=enterPackage(group.Dir,group.Name);if err!=nil {b.Fatal(err)};b.Cleanup(restore);bench.F(b)
 }) } }
}
'''
main_source=dest/'suite_test.go';write_if_changed(main_source,harness)
overlay[str(ROOT/'pkg/monolithpilot/suite_test.go')]=str(main_source)
write_if_changed(dest/'overlay.json',json.dumps({'Replace':overlay},indent=2))
write_if_changed(dest/'groups.json',json.dumps(groups,indent=2))
excluded={pkg:reason for pkg,reason in excluded.items() if pkg in mains}
write_if_changed(excluded_path,json.dumps(excluded,indent=2))
print(json.dumps({'merged_packages':len(groups),'native_packages':len(excluded),'overlay_files':len(overlay),'bridge_packages':len(all_bridges)}))
