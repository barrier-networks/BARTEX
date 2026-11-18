package db

import "testing"

func TestHarnessProfileRoundTrip(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skip("no PG for test:", err)
	}
	defer d.Close()

	p := &HarnessProfile{
		Name:                 "codex-daybreak",
		Kind:                 "codex",
		Command:              "codex",
		Args:                 []string{"exec", "-c", "daybreak=true", "{{PROMPT}}"},
		Env:                  map[string]string{"OPENAI_API_KEY": "sk-secret"},
		MCPTransport:         "stdio",
		DisableNativeSandbox: true,
		Daybreak:             true,
		MaxDurationSeconds:   900,
	}
	id, err := d.SaveHarnessProfile(p)
	if err != nil {
		t.Fatal(err)
	}

	// Read back with env decrypted.
	got, err := d.HarnessProfileByID(id)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.Command != "codex" || got.Kind != "codex" || !got.Daybreak || got.MaxDurationSeconds != 900 {
		t.Fatalf("fields round-tripped wrong: %+v", got)
	}
	if len(got.Args) != 4 || got.Args[0] != "exec" {
		t.Fatalf("args: %v", got.Args)
	}
	if got.Env["OPENAI_API_KEY"] != "sk-secret" {
		t.Fatalf("env not decrypted: %v", got.Env)
	}

	// The sealed env is not stored in clear.
	var raw string
	if err := d.QueryRow(`SELECT env::text FROM harness_profiles WHERE id=$1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw == "" || containsPlain(raw, "sk-secret") {
		t.Fatalf("env stored in clear: %s", raw)
	}

	// List returns only env key names, never values.
	list, err := d.ListHarnessProfiles()
	if err != nil {
		t.Fatal(err)
	}
	var found *HarnessProfile
	for _, lp := range list {
		if lp.ID == id {
			found = lp
		}
	}
	if found == nil {
		t.Fatal("profile missing from list")
	}
	if len(found.EnvKeys) != 1 || found.EnvKeys[0] != "OPENAI_API_KEY" {
		t.Fatalf("env keys: %v", found.EnvKeys)
	}
	if found.Env != nil {
		t.Error("list must not decrypt env values")
	}

	// Update without env keeps the stored (sealed) env.
	got.Env = nil
	got.Command = "codex2"
	if _, err := d.SaveHarnessProfile(got); err != nil {
		t.Fatal(err)
	}
	after, _ := d.HarnessProfileByID(id)
	if after.Command != "codex2" || after.Env["OPENAI_API_KEY"] != "sk-secret" {
		t.Fatalf("update dropped env or command: %+v", after)
	}

	if err := d.DeleteHarnessProfile(id); err != nil {
		t.Fatal(err)
	}
	gone, _ := d.HarnessProfileByID(id)
	if gone != nil {
		t.Fatal("profile not deleted")
	}
}

func containsPlain(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func TestDefaultHarnessProfileID(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skip("no PG for test:", err)
	}
	defer d.Close()

	// Unset by default.
	if id, err := d.DefaultHarnessProfileID(); err != nil || id != nil {
		t.Fatalf("default should be unset: id=%v err=%v", id, err)
	}
	// Setting a non-existent profile is rejected.
	bad := int64(999999)
	if err := d.SetDefaultHarnessProfileID(&bad); err == nil {
		t.Fatal("setting a missing profile id should fail")
	}
	// Create a profile, set it as default, read it back.
	pid, err := d.SaveHarnessProfile(&HarnessProfile{Name: "dflt", Kind: "codex", Command: "codex", MCPTransport: "stdio"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetDefaultHarnessProfileID(&pid); err != nil {
		t.Fatal(err)
	}
	if id, err := d.DefaultHarnessProfileID(); err != nil || id == nil || *id != pid {
		t.Fatalf("default = %v (err %v), want %d", id, err, pid)
	}
	// Clearing returns to unset.
	if err := d.SetDefaultHarnessProfileID(nil); err != nil {
		t.Fatal(err)
	}
	if id, _ := d.DefaultHarnessProfileID(); id != nil {
		t.Fatalf("default should be cleared, got %v", id)
	}
	_ = d.DeleteHarnessProfile(pid)
}
