package agent_messages_test

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func installProviderChild(t *testing.T, dir string) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("controlled child requires existing node installation")
	}
	provider := filepath.Join(dir, "provider")
	if err := os.Mkdir(provider, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(provider, "child.cjs")
	if err := os.WriteFile(script, []byte(providerChild), 0o600); err != nil {
		t.Fatal(err)
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	name, launcher := "codex", "#!/bin/sh\nexec "+quote(node)+" "+quote(script)+" \"$@\"\n"
	if runtime.GOOS == "windows" {
		name, launcher = "codex.cmd", "@echo off\r\n\""+node+"\" \"%~dp0child.cjs\" %*\r\nexit /b %errorlevel%\r\n"
	}
	if err := os.WriteFile(filepath.Join(provider, name), []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
	return provider
}

func (f *artifactFixture) invoke(t *testing.T, server, id string, caller map[string]string) map[string]string {
	t.Helper()
	status, body := f.http(t, server, http.MethodPost, "/factory-sessions", map[string]string{"folderPath": f.dir}, nil)
	var opened api.OpenFactorySessionResponse
	if status != http.StatusOK || json.Unmarshal(body, &opened) != nil || opened.Session == nil {
		t.Fatal("compiled host did not open an explicit Factory Session")
	}
	document := map[string]any{"requestId": id + "-invoke", "workerSessionId": id, "execution": map[string]any{
		"workstationName": "direct", "workingDirectory": f.dir, "factorySessionId": opened.Session.Id,
		"workerType": "direct-worker", "runnerId": "codex", "executorProvider": "codex", "modelProvider": "codex", "model": "controlled-model", "userMessage": "controlled input",
		"dispatch": map[string]string{"dispatchId": id + "-attempt", "workstationName": "direct", "workerType": "direct-worker"},
	}}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.dir, id+"-execution.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	f.cli(t, callerEnvironment(f.env, caller), "--json", "--remote", "--server", server, "worker-sessions", "invoke", "--execution", path, "--async")
	return f.workerCredentials(t, id)
}

func (f *artifactFixture) workerCredentials(t *testing.T, id string) map[string]string {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(filepath.Join(f.dir, id+"-credentials.json"))
		if err == nil {
			var credentials map[string]string
			if json.Unmarshal(data, &credentials) == nil && credentials["YOU_WORKER_SESSION_ID"] == id && credentials["YOU_WORKER_SESSION_TOKEN"] != "" {
				return credentials
			}
		}
		select {
		case <-ticker.C:
		case <-f.ctx.Done():
			t.Fatal("controlled child did not receive execution credentials")
		}
	}
}

// This proves the continuation prerequisite and fresh execution credentials.
// Durable inbox pull with the new caller remains a separate unproved edge.
func (f *artifactFixture) continueCallerAfterRestart(t *testing.T, server string, previous map[string]string) {
	t.Helper()
	request := map[string]string{"requestId": "restart-continue", "successorWorkerSessionId": "lane-next", "followUpInput": "controlled next input"}
	status, body := f.http(t, server, http.MethodPost, "/worker-sessions/lane/continue", request, nil)
	if status != http.StatusAccepted {
		var failure api.ErrorResponse
		_ = json.Unmarshal(body, &failure)
		t.Fatalf("fresh continuation: status=%d code=%s", status, failure.Code)
	}
	next := f.workerCredentials(t, "lane-next")
	if next["YOU_WORKER_SESSION_TOKEN"] == previous["YOU_WORKER_SESSION_TOKEN"] {
		t.Fatal("restart continuation reused the previous execution token")
	}
	status, body = f.http(t, server, http.MethodGet, "/worker-sessions/lane-next", nil, nil)
	var observation api.WorkerSessionObservation
	if status != http.StatusOK || json.Unmarshal(body, &observation) != nil || observation.State != "RUNNING" ||
		observation.PredecessorWorkerSessionId == nil || *observation.PredecessorWorkerSessionId != "lane" {
		t.Fatal("restart continuation lost running state or predecessor identity")
	}
	f.releaseCaller(t, server, "lane-next")
}

func (f *artifactFixture) releaseCaller(t *testing.T, server, id string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, id+"-release"), []byte("complete"), 0o600); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, bytes := f.http(t, server, http.MethodGet, "/worker-sessions/"+id, nil, nil)
		var observation api.WorkerSessionObservation
		if status == http.StatusOK && json.Unmarshal(bytes, &observation) == nil && observation.State == "COMPLETED" {
			return
		}
		select {
		case <-ticker.C:
		case <-f.ctx.Done():
			t.Fatalf("controlled caller did not complete: state=%s", observation.State)
		}
	}
}

const providerChild = `const fs = require('fs');
const path = require('path');
if (!process.argv.slice(2).includes('exec')) { console.log('codex controlled 1.0'); process.exit(0); }
process.stdin.resume();
process.stdin.on('end', () => {
 const id = process.env.YOU_WORKER_SESSION_ID;
 const target = path.join(process.env.AM_T5_CREDENTIAL_ROOT, id+'-credentials.json');
 const credentials = {YOU_WORKER_SESSION_ID:id, YOU_WORKER_SESSION_TOKEN:process.env.YOU_WORKER_SESSION_TOKEN, YOU_SERVER:process.env.YOU_SERVER};
 fs.writeFileSync(target+'.tmp', JSON.stringify(credentials));
 fs.renameSync(target+'.tmp', target);
 const args = process.argv.slice(2);
 const resume = args.indexOf('resume');
 const thread = resume >= 0 ? args[resume+1] : 'controlled-'+id;
 console.log(JSON.stringify({type:'thread.started',thread_id:thread}));
 const parent = process.ppid;
 const lifetime = setTimeout(() => process.exit(1), 80000);
 const interval = setInterval(() => {
  if (fs.existsSync(path.join(process.env.AM_T5_CREDENTIAL_ROOT,id+'-release'))) {
   console.log(JSON.stringify({type:'item.completed',item:{id:'final',type:'agent_message',text:'controlled worker complete'}}));
   console.log(JSON.stringify({type:'turn.completed',usage:{input_tokens:1,output_tokens:1}}));
   clearInterval(interval); clearTimeout(lifetime);
  } else if (process.platform === 'win32') { try { process.kill(parent,0); } catch { process.exit(0); } }
 }, 50);
});
`
