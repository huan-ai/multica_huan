package handler

import (
	"testing"
)

func TestDDDStageLookup_ExistingStage(t *testing.T) {
	def := dddStageLookup(5)
	if def == nil {
		t.Fatal("expected stage 5 to exist")
	}
	if def.SkillName != "ddd-strategic-design" {
		t.Errorf("stage 5 skill = %q, want ddd-strategic-design", def.SkillName)
	}
	if def.GateName != "g05" {
		t.Errorf("stage 5 gate = %q, want g05", def.GateName)
	}
}

func TestDDDStageLookup_NonExistentStage(t *testing.T) {
	def := dddStageLookup(99)
	if def != nil {
		t.Fatalf("expected nil for nonexistent stage 99, got %+v", def)
	}
}

func TestDDDStageDefinitions_Complete(t *testing.T) {
	if len(dddAllStages) != 14 {
		t.Fatalf("expected 14 stage definitions, got %d", len(dddAllStages))
	}
	for i, def := range dddAllStages {
		if def.Stage != int32(i) {
			t.Errorf("stage %d: Stage field = %d, want %d", i, def.Stage, i)
		}
		if def.Title == "" {
			t.Errorf("stage %d: Title is empty", i)
		}
		if def.SkillName == "" {
			t.Errorf("stage %d: SkillName is empty", i)
		}
		if def.AppliesTo == "" {
			t.Errorf("stage %d: AppliesTo is empty", i)
		}
	}
}

func TestDDDStageDefinitions_AppliesTo_Valid(t *testing.T) {
	validValues := map[string]bool{
		"all":                 true,
		"existing_only":      true,
		"existing_light_only": true,
		"full_only":          true,
	}
	for _, def := range dddAllStages {
		if !validValues[def.AppliesTo] {
			t.Errorf("stage %d (%s): invalid AppliesTo %q", def.Stage, def.SkillName, def.AppliesTo)
		}
	}
}

func TestDDDStageDefinitions_GreenfieldPath(t *testing.T) {
	// Greenfield: all + full_only stages should be included.
	// Expected: 0,1,2,5,6,7,8,9,10,11,12,13 (12 stages, no 3/4)
	included := map[int32]bool{}
	for _, def := range dddAllStages {
		switch def.AppliesTo {
		case "all", "full_only":
			included[def.Stage] = true
		}
	}
	expected := []int32{0, 1, 2, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	if len(included) != len(expected) {
		t.Fatalf("greenfield: expected %d stages, got %d", len(expected), len(included))
	}
	for _, s := range expected {
		if !included[s] {
			t.Errorf("greenfield: stage %d should be included", s)
		}
	}
	// Stages 3 and 4 must NOT be included
	for _, s := range []int32{3, 4} {
		if included[s] {
			t.Errorf("greenfield: stage %d should NOT be included", s)
		}
	}
}

func TestDDDStageDefinitions_ExistingFullPath(t *testing.T) {
	// Existing+full: all + existing_only + full_only (not existing_light_only)
	// Expected: 0,1,2,3,5,6,7,8,9,10,11,12,13 (13 stages, no 4)
	included := map[int32]bool{}
	for _, def := range dddAllStages {
		switch def.AppliesTo {
		case "all", "existing_only", "full_only":
			included[def.Stage] = true
		}
	}
	expected := []int32{0, 1, 2, 3, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	if len(included) != len(expected) {
		t.Fatalf("existing+full: expected %d stages, got %d", len(expected), len(included))
	}
	for _, s := range expected {
		if !included[s] {
			t.Errorf("existing+full: stage %d should be included", s)
		}
	}
	if included[4] {
		t.Error("existing+full: stage 4 (light-path-design) should NOT be included")
	}
}

func TestDDDStageDefinitions_ExistingLightPath(t *testing.T) {
	// Existing+light: all + existing_only + existing_light_only
	// full_only stages (5-9) are created but marked cancelled (tested separately)
	// Included as active: 0,1,2,3,4,10,11,12,13
	active := map[int32]bool{}
	for _, def := range dddAllStages {
		switch def.AppliesTo {
		case "all", "existing_only", "existing_light_only":
			active[def.Stage] = true
		}
	}
	expected := []int32{0, 1, 2, 3, 4, 10, 11, 12, 13}
	if len(active) != len(expected) {
		t.Fatalf("existing+light active: expected %d stages, got %d", len(expected), len(active))
	}
	for _, s := range expected {
		if !active[s] {
			t.Errorf("existing+light: stage %d should be active", s)
		}
	}
}

func TestDDDStageDefinitions_ExistingLightPath_SkippedStages(t *testing.T) {
	// Stages 5-9 should be created as cancelled in light path.
	skipped := []int32{5, 6, 7, 8, 9}
	for _, s := range skipped {
		def := dddStageLookup(s)
		if def == nil {
			t.Fatalf("stage %d not found", s)
		}
		if def.AppliesTo != "full_only" {
			t.Errorf("stage %d: AppliesTo = %q, want full_only (should be skipped in light path)", s, def.AppliesTo)
		}
	}
}

func TestDDDStageDefinitions_LegacyPath(t *testing.T) {
	// Legacy: same as greenfield (all + full_only)
	included := map[int32]bool{}
	for _, def := range dddAllStages {
		switch def.AppliesTo {
		case "all", "full_only":
			included[def.Stage] = true
		}
	}
	expected := []int32{0, 1, 2, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	if len(included) != len(expected) {
		t.Fatalf("legacy: expected %d stages, got %d", len(expected), len(included))
	}
}

func TestDDDStageDefinitions_JointGate_G09(t *testing.T) {
	// Stages 7, 8, 9 share gate g09.
	for _, s := range []int32{7, 8, 9} {
		def := dddStageLookup(s)
		if def == nil {
			t.Fatalf("stage %d not found", s)
		}
		if def.GateName != "g09" {
			t.Errorf("stage %d: GateName = %q, want g09 (joint gate)", s, def.GateName)
		}
	}
}

func TestDDDStageDefinitions_ParallelStages(t *testing.T) {
	// Stages 7 (tactical modeling) and 8 (web design) run in parallel.
	s7 := dddStageLookup(7)
	s8 := dddStageLookup(8)
	if s7 == nil || s8 == nil {
		t.Fatal("stages 7 and 8 must exist")
	}
	if s7.SkillName != "ddd-tactical-modeling" {
		t.Errorf("stage 7: skill = %q, want ddd-tactical-modeling", s7.SkillName)
	}
	if s8.SkillName != "ddd-web-design" {
		t.Errorf("stage 8: skill = %q, want ddd-web-design", s8.SkillName)
	}
	// Both should be full_only and share gate g09
	if s7.AppliesTo != "full_only" || s8.AppliesTo != "full_only" {
		t.Error("parallel stages 7/8 must both be full_only")
	}
	if s7.GateName != "g09" || s8.GateName != "g09" {
		t.Error("parallel stages 7/8 must share gate g09")
	}
}

func TestDDDStageDefinitions_Stage11_NoGate(t *testing.T) {
	// Stage 11 (implementation execution) has no human gate.
	def := dddStageLookup(11)
	if def == nil {
		t.Fatal("stage 11 must exist")
	}
	if def.GateName != "" {
		t.Errorf("stage 11: GateName = %q, want empty (no human gate)", def.GateName)
	}
}

func TestDDDStageDefinitions_InitialStagesAreFirst3(t *testing.T) {
	// InitDDDFlow creates stages 0-2 initially. Verify they are the first 3.
	initial := dddAllStages[:3]
	if len(initial) != 3 {
		t.Fatalf("initial stages: expected 3, got %d", len(initial))
	}
	for i, def := range initial {
		if def.Stage != int32(i) {
			t.Errorf("initial stage %d: Stage = %d", i, def.Stage)
		}
		if def.AppliesTo != "all" {
			t.Errorf("initial stage %d: AppliesTo = %q, want all", i, def.AppliesTo)
		}
	}
}

func TestDDDStageDefinitions_UniqueSkillNames(t *testing.T) {
	seen := map[string]int32{}
	for _, def := range dddAllStages {
		if prev, ok := seen[def.SkillName]; ok {
			t.Errorf("duplicate SkillName %q: stages %d and %d", def.SkillName, prev, def.Stage)
		}
		seen[def.SkillName] = def.Stage
	}
}

func TestDDDStageDefinitions_StagesAreSequential(t *testing.T) {
	for i, def := range dddAllStages {
		if def.Stage != int32(i) {
			t.Errorf("stage at index %d has Stage=%d, want %d (stages must be sequential)", i, def.Stage, i)
		}
	}
}

// --- Enforcement logic tests ---

func TestJoinWithComma(t *testing.T) {
	tests := []struct {
		items []string
		want  string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a、b"},
		{[]string{"阶段 1", "阶段 2", "阶段 3"}, "阶段 1、阶段 2、阶段 3"},
	}
	for _, tt := range tests {
		got := joinWithComma(tt.items)
		if got != tt.want {
			t.Errorf("joinWithComma(%v) = %q, want %q", tt.items, got, tt.want)
		}
	}
}
