package k8sclient

import (
	"testing"

	scenarioconfig "github.com/randsw/cascadescenariocontroller/cascadescenario"
)

func TestJobStatusConstants(t *testing.T) {
	// Verify JobStatus enum values
	if int(notStarted) != 0 {
		t.Errorf("Expected notStarted=0, got %d", notStarted)
	}
	if int(Running) != 1 {
		t.Errorf("Expected Running=1, got %d", Running)
	}
	if int(Succeeded) != 2 {
		t.Errorf("Expected Succeeded=2, got %d", Succeeded)
	}
	if int(Failed) != 3 {
		t.Errorf("Expected Failed=3, got %d", Failed)
	}
}

func TestJobStatus_StringRepresentation(t *testing.T) {
	tests := []struct {
		name     string
		status   JobStatus
		expected int
	}{
		{"notStarted", notStarted, 0},
		{"Running", Running, 1},
		{"Succeeded", Succeeded, 2},
		{"Failed", Failed, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.status) != tt.expected {
				t.Errorf("Expected %s=%d, got %d", tt.name, tt.expected, tt.status)
			}
		})
	}
}

func TestK8sScenarioConfig_Defaults(t *testing.T) {
	config := K8sScenarioConfig{
		ScenarioNamespace: "default",
		ScenarioName:      "test-scenario",
		S3Path:            "s3://bucket/path.tgz",
		TUUID:             "abc-123-def",
		OutMinioAddress:   "http://minio:9000/",
		SName:             "source-name",
		Ob:                "object-name",
	}

	if config.ScenarioNamespace != "default" {
		t.Errorf("Expected ScenarioNamespace 'default', got '%s'", config.ScenarioNamespace)
	}
	if config.ScenarioName != "test-scenario" {
		t.Errorf("Expected ScenarioName 'test-scenario', got '%s'", config.ScenarioName)
	}
	if config.S3Path != "s3://bucket/path.tgz" {
		t.Errorf("Expected S3Path 's3://bucket/path.tgz', got '%s'", config.S3Path)
	}
	if config.TUUID != "abc-123-def" {
		t.Errorf("Expected TUUID 'abc-123-def', got '%s'", config.TUUID)
	}
	if config.OutMinioAddress != "http://minio:9000/" {
		t.Errorf("Expected OutMinioAddress 'http://minio:9000/', got '%s'", config.OutMinioAddress)
	}
	if config.SName != "source-name" {
		t.Errorf("Expected SName 'source-name', got '%s'", config.SName)
	}
	if config.Ob != "object-name" {
		t.Errorf("Expected Ob 'object-name', got '%s'", config.Ob)
	}
}

func TestS3TransferPath_Defaults(t *testing.T) {
	s3path := S3TransferPath{
		StageNum:        0,
		Path:            "s3://bucket/incoming/file.tgz",
		IsLastStage:     false,
		UUID:            "test-uuid-12345",
		OutMinioAddress: "http://minio:9000/",
	}

	if s3path.StageNum != 0 {
		t.Errorf("Expected StageNum 0, got %d", s3path.StageNum)
	}
	if s3path.Path != "s3://bucket/incoming/file.tgz" {
		t.Errorf("Expected Path 's3://bucket/incoming/file.tgz', got '%s'", s3path.Path)
	}
	if s3path.IsLastStage {
		t.Error("Expected IsLastStage false")
	}
	if s3path.UUID != "test-uuid-12345" {
		t.Errorf("Expected UUID 'test-uuid-12345', got '%s'", s3path.UUID)
	}
	if s3path.OutMinioAddress != "http://minio:9000/" {
		t.Errorf("Expected OutMinioAddress 'http://minio:9000/', got '%s'", s3path.OutMinioAddress)
	}
}

func TestS3TransferPath_LastStage(t *testing.T) {
	s3path := S3TransferPath{
		StageNum:    2,
		Path:        "s3://bucket/stage-2.zip",
		IsLastStage: true,
		UUID:        "uuid-67890",
	}

	if !s3path.IsLastStage {
		t.Error("Expected IsLastStage true")
	}
	if s3path.StageNum != 2 {
		t.Errorf("Expected StageNum 2, got %d", s3path.StageNum)
	}
}

func TestS3TransferPath_StageProgression(t *testing.T) {
	// Simulate stage progression
	s3path := &S3TransferPath{
		UUID:            "progression-uuid",
		OutMinioAddress: "http://minio:9000/",
	}

	stages := []struct {
		stageNum    int
		isLastStage bool
		path        string
	}{
		{0, false, "s3://bucket/input.tgz"},
		{1, false, "http://minio:9000/scenario/progression-uuid/progression-uuid-stage-1.zip"},
		{2, true, "http://minio:9000/scenario/progression-uuid/progression-uuid-stage-2.zip"},
	}

	for _, stage := range stages {
		s3path.StageNum = stage.stageNum
		s3path.IsLastStage = stage.isLastStage

		if s3path.StageNum != stage.stageNum {
			t.Errorf("Stage %d: expected StageNum %d, got %d", stage.stageNum, stage.stageNum, s3path.StageNum)
		}
		if s3path.IsLastStage != stage.isLastStage {
			t.Errorf("Stage %d: expected IsLastStage %v, got %v", stage.stageNum, stage.isLastStage, s3path.IsLastStage)
		}
	}
}

func TestK8sScenarioConfig_WithCascadeScenarios(t *testing.T) {
	config := K8sScenarioConfig{
		ScenarioNamespace: "cascade-operator",
		ScenarioName:      "image-processing",
		OutMinioAddress:   "http://minio:9000/out/",
	}

	scenario := scenarioconfig.CascadeScenarios{
		ModuleName:    "grayscale",
		Configuration: map[string]string{"key": "value"},
	}

	if config.ScenarioNamespace != "cascade-operator" {
		t.Errorf("Expected namespace 'cascade-operator', got '%s'", config.ScenarioNamespace)
	}
	if scenario.ModuleName != "grayscale" {
		t.Errorf("Expected module 'grayscale', got '%s'", scenario.ModuleName)
	}
	if scenario.Configuration["key"] != "value" {
		t.Errorf("Expected config key 'value', got '%s'", scenario.Configuration["key"])
	}
}