package cascadescenario

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestReadConfigJSON(t *testing.T) {
	expectedModulesNum := 3
	filename := "./test/test_fail_first.json"
	modules := ReadConfigJSON(filename)
	if len(modules) != expectedModulesNum {
		t.Errorf("Output %d not equal to expected %d", len(modules), expectedModulesNum)
		return
	}
	t.Log("Number of modules is correct")
}

func TestReadConfigJSONBackoffLimit(t *testing.T) {
	expectedBackoffLimit := 0
	filename := "./test/test_fail_first.json"
	modules := ReadConfigJSON(filename)
	if *modules[0].BackoffLimit != int32(expectedBackoffLimit) {
		t.Errorf("Output %d not equal to expected %d", *modules[0].BackoffLimit, expectedBackoffLimit)
		return
	}
	t.Log("Scenario controller job backofflimit is correct")
}

func TestReadConfigJSONImageName(t *testing.T) {
	firstModuleImageName := "ghcr.io/randsw/grayscale:0.1.1"
	filename := "./test/test_fail_first.json"
	modules := ReadConfigJSON(filename)
	if modules[0].Template.Spec.Containers[0].Image != firstModuleImageName {
		t.Errorf("Output %q not equal to expected %q", modules[0].Template.Spec.Containers[0].Image, firstModuleImageName)
		return
	}
	t.Log("First module image name is correct")
}

func TestReadConfigJSON_SuccessFile(t *testing.T) {
	expectedModulesNum := 3
	filename := "./test/test_success.json"
	modules := ReadConfigJSON(filename)
	if len(modules) != expectedModulesNum {
		t.Errorf("Output %d not equal to expected %d", len(modules), expectedModulesNum)
		return
	}
	t.Log("Number of modules in success file is correct")
}

func TestReadConfigJSON_ModuleNames(t *testing.T) {
	filename := "./test/test_fail_first.json"
	modules := ReadConfigJSON(filename)

	expectedNames := []string{"grayscale", "fail", "diff"}
	for i, expected := range expectedNames {
		if modules[i].ModuleName != expected {
			t.Errorf("Module %d: expected name '%s', got '%s'", i, expected, modules[i].ModuleName)
		}
	}
}

func TestReadConfigJSON_Configuration(t *testing.T) {
	filename := "./test/test_fail_first.json"
	modules := ReadConfigJSON(filename)

	// First module should have 4 configuration entries
	if len(modules[0].Configuration) != 4 {
		t.Errorf("Expected 4 config entries in first module, got %d", len(modules[0].Configuration))
	}

	// Check specific config values
	if modules[0].Configuration["foo"] != "bar" {
		t.Errorf("Expected foo=bar, got foo=%s", modules[0].Configuration["foo"])
	}
	if modules[0].Configuration["spamm"] != "eggs" {
		t.Errorf("Expected spamm=eggs, got spamm=%s", modules[0].Configuration["spamm"])
	}
}

func TestReadConfigJSON_SecondModuleConfig(t *testing.T) {
	filename := "./test/test_fail_first.json"
	modules := ReadConfigJSON(filename)

	// Second module has different configuration
	if modules[1].Configuration["eggs"] != "spamm" {
		t.Errorf("Expected eggs=spamm, got eggs=%s", modules[1].Configuration["eggs"])
	}
	if modules[1].Configuration["thresh"] != "128" {
		t.Errorf("Expected thresh=128, got thresh=%s", modules[1].Configuration["thresh"])
	}
}

func TestReadConfigJSON_TemplateSpec(t *testing.T) {
	filename := "./test/test_success.json"
	modules := ReadConfigJSON(filename)

	// Verify template spec is populated
	if len(modules[0].Template.Spec.Containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(modules[0].Template.Spec.Containers))
	}

	// Verify restart policy
	if modules[0].Template.Spec.RestartPolicy != corev1.RestartPolicyOnFailure {
		t.Errorf("Expected RestartPolicy OnFailure, got %s", modules[0].Template.Spec.RestartPolicy)
	}
}

func TestReadConfigJSON_SecondModuleCommand(t *testing.T) {
	filename := "./test/test_fail_first.json"
	modules := ReadConfigJSON(filename)

	// Second module (fail) has a command and args
	if modules[1].Template.Spec.Containers[0].Name != "fail" {
		t.Errorf("Expected container name 'fail', got '%s'", modules[1].Template.Spec.Containers[0].Name)
	}
	if len(modules[1].Template.Spec.Containers[0].Command) != 3 {
		t.Errorf("Expected 3 command elements, got %d", len(modules[1].Template.Spec.Containers[0].Command))
	}
	if len(modules[1].Template.Spec.Containers[0].Args) != 1 {
		t.Errorf("Expected 1 arg, got %d", len(modules[1].Template.Spec.Containers[0].Args))
	}
}

func TestReadConfigJSON_AllModulesHaveNames(t *testing.T) {
	filename := "./test/test_success.json"
	modules := ReadConfigJSON(filename)

	for i, module := range modules {
		if module.ModuleName == "" {
			t.Errorf("Module %d has empty name", i)
		}
		if len(module.Configuration) == 0 {
			t.Errorf("Module %d (%s) has no configuration", i, module.ModuleName)
		}
	}
}

func TestReadConfigJSON_SuccessBackoffLimits(t *testing.T) {
	filename := "./test/test_success.json"
	modules := ReadConfigJSON(filename)

	for i, module := range modules {
		if module.BackoffLimit == nil {
			t.Errorf("Module %d (%s) has nil BackoffLimit", i, module.ModuleName)
		} else if *module.BackoffLimit != 0 {
			t.Errorf("Module %d (%s): expected BackoffLimit 0, got %d", i, module.ModuleName, *module.BackoffLimit)
		}
	}
}

func TestCascadeScenarios_StructFields(t *testing.T) {
	backoff := int32(3)
	deadline := int64(300)
	ttl := int32(60)

	scenario := CascadeScenarios{
		ModuleName:              "test-module",
		Configuration:           map[string]string{"key": "value"},
		ActiveDeadlineSeconds:   &deadline,
		BackoffLimit:            &backoff,
		TTLSecondsAfterFinished: &ttl,
		Template: corev1.PodTemplateSpec{},
	}

	if scenario.ModuleName != "test-module" {
		t.Errorf("Expected ModuleName 'test-module', got '%s'", scenario.ModuleName)
	}
	if *scenario.ActiveDeadlineSeconds != 300 {
		t.Errorf("Expected ActiveDeadlineSeconds 300, got %d", *scenario.ActiveDeadlineSeconds)
	}
	if *scenario.BackoffLimit != 3 {
		t.Errorf("Expected BackoffLimit 3, got %d", *scenario.BackoffLimit)
	}
	if *scenario.TTLSecondsAfterFinished != 60 {
		t.Errorf("Expected TTLSecondsAfterFinished 60, got %d", *scenario.TTLSecondsAfterFinished)
	}
}

// NOTE: Error case tests for ReadConfigJSON (non-existent file, invalid JSON)
// are not possible because ReadConfigJSON calls os.Exit(1) on error,
// which would terminate the test process. This is a known design limitation
// that should be addressed by returning errors instead of calling os.Exit.
