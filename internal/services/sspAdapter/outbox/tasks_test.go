package outbox

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPublisherTasksDurableLeasesAndBoundedClaims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"a", "b", "c"} {
		if e = s.SaveTask(id, json.RawMessage(`{"trusted":true}`)); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.SaveTask("a", json.RawMessage(`{"trusted":false}`)); e != ErrEventConflict {
		t.Fatal(e)
	}
	now := time.Now()
	tasks, e := s.ClaimTasks(now, 2, time.Second)
	if e != nil || len(tasks) != 2 {
		t.Fatal(tasks, e)
	}
	if n, _ := s.TaskCount(); n != 3 {
		t.Fatal(n)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	rest, e := s.ClaimTasks(now, 2, time.Second)
	if e != nil || len(rest) != 1 {
		t.Fatal(rest, e)
	}
	recovered, e := s.ClaimTasks(now.Add(2*time.Second), 8, time.Second)
	if e != nil || len(recovered) != 3 {
		t.Fatal(recovered, e)
	}
	if e = s.FinishTask(tasks[0], time.Time{}); e == nil {
		t.Fatal("stale ACK deleted a new lease")
	}
	for _, task := range recovered {
		if e = s.FinishTask(task, time.Time{}); e != nil {
			t.Fatal(e)
		}
	}
	if n, _ := s.TaskCount(); n != 0 {
		t.Fatal(n)
	}
	// A replaying publisher task never affects ADV recovery gating.
	if records, e := s.List(); e != nil || len(records) != 0 {
		t.Fatal(records, e)
	}
}

func TestPublisherTasksSurviveSIGKILL(t *testing.T) {
	if path := os.Getenv("TEST_PUB_BURL_CRASH_PATH"); path != "" {
		box, e := Open(path)
		if e != nil {
			t.Fatal(e)
		}
		if e = box.SaveTask("winner", json.RawMessage(`{"uuid":"same","trusted":true}`)); e != nil {
			t.Fatal(e)
		}
		tasks, e := box.ClaimTasks(time.Now(), 1, time.Second)
		if e != nil || len(tasks) != 1 {
			t.Fatal(tasks, e)
		}
		if e = os.WriteFile(path+".ready", []byte("leased"), 0600); e != nil {
			t.Fatal(e)
		}
		time.Sleep(time.Hour)
		return
	}
	path := filepath.Join(t.TempDir(), "hard-crash.db")
	cmd := exec.Command(os.Args[0], "-test.run=^TestPublisherTasksSurviveSIGKILL$")
	cmd.Env = append(os.Environ(), "TEST_PUB_BURL_CRASH_PATH="+path)
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		if _, e := os.Stat(path + ".ready"); e == nil {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("child did not persist active task")
	}
	if e := cmd.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	cmd.Wait()
	box, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	tasks, e := box.ClaimTasks(time.Now().Add(2*time.Second), 1, time.Second)
	if e != nil || len(tasks) != 1 || string(tasks[0].Payload) != `{"uuid":"same","trusted":true}` || tasks[0].Attempts != 2 {
		t.Fatal("hard restart lost job", tasks, e)
	}
}
