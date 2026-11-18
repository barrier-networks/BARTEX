package db

import "testing"

func TestSeedBuiltinTaskTemplates(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skip("no PG for test:", err)
	}
	defer d.Close()

	// Open already ran the seed; the curated templates must be present by name.
	list, err := d.ListTaskTemplates()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]bool{}
	for _, tpl := range list {
		byName[tpl.Name] = true
	}
	for _, want := range []string{"AWS Cloud Assessment (Pacu)", "External Infrastructure", "Web Application Assessment", "Attack Surface Discovery (bbot)"} {
		if !byName[want] {
			t.Errorf("seeded template %q missing", want)
		}
	}

	before := len(list)

	// Re-seed is a no-op (flag gates it): no duplicates, no error.
	if err := d.SeedBuiltinTaskTemplates(); err != nil {
		t.Fatal(err)
	}
	after, _ := d.ListTaskTemplates()
	if len(after) != before {
		t.Fatalf("re-seed changed template count: %d -> %d", before, len(after))
	}

	// A deleted built-in is NOT resurrected on re-seed (the flag is already done).
	var victim *TaskTemplate
	for _, tpl := range after {
		if tpl.Name == "Attack Surface Discovery (bbot)" {
			victim = tpl
		}
	}
	if victim == nil {
		t.Fatal("expected template to delete")
	}
	if _, err := d.DeleteTaskTemplate(victim.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.SeedBuiltinTaskTemplates(); err != nil {
		t.Fatal(err)
	}
	final, _ := d.ListTaskTemplates()
	for _, tpl := range final {
		if tpl.ID == victim.ID || tpl.Name == "Attack Surface Discovery (bbot)" {
			t.Error("deleted built-in template was resurrected on re-seed")
		}
	}
}
