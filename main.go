package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	//"strings"
	"encoding/json"

	scenarioconfig "github.com/randsw/cascadescenariocontroller/cascadescenario"
	k8sClient "github.com/randsw/cascadescenariocontroller/k8sclient"
	"github.com/randsw/cascadescenariocontroller/logger"
	"go.uber.org/zap"
)

const (
	notStarted k8sClient.JobStatus = iota
	Running
	Succeeded
	Failed
)

type k8sScenarioConfig struct {
	ScenarioNamespace string
	ScenarioName      string
	s3PackagePath     string
}

type Payload struct {
	Timestamp     int64
	Source_ID     string
	Sub_source_ID string
	Path          string
}

func main() {
	//Loger Initialization
	logger.InitLogger()
	defer logger.CloseLogger()
	//Get Config from file mounted in tmp folder
	configFilename := "/tmp/configuration"

	//configFilename := "cascadescenario/test/test_success.json"

	//configFilename := "cascadescenario/test/test_fail_first.json"

	CascadeScenatioConfig := scenarioconfig.ReadConfigJSON(configFilename)
	//Get pod namespace
	jobNamespace := "image-process"
	if envvar := os.Getenv("POD_NAMESPACE"); len(envvar) > 0 {
		jobNamespace = envvar
	}
	//Get status server address
	statusServerAddress := "http://127.0.0.1:8000"
	if envvar := os.Getenv("STATUS_SERVER"); len(envvar) > 0 {
		statusServerAddress = envvar
	}
	//Get scenario name
	scenarioName := "Test-image-processing"
	if envvar := os.Getenv("SCENARIO_NAME"); len(envvar) > 0 {
		scenarioName = envvar
	}
	//Get Brocker Address
	brockerAddress := "192.168.49.2:30092"
	if envvar := os.Getenv("BROCKER_ADDRESS"); len(envvar) > 0 {
		brockerAddress = envvar
	}
	//Get Topic name
	topic := "source"
	if envvar := os.Getenv("TOPIC"); len(envvar) > 0 {
		topic = envvar
	}

	//Get consumer group
	source_ID := "source1_X5"
	if envvar := os.Getenv("SOURCE_ID"); len(envvar) > 0 {
		source_ID = envvar
	}
	//Create channel for signal
	cancelChan := make(chan os.Signal, 1)
	// catch SIGETRM or SIGINTERRUPT
	signal.Notify(cancelChan, syscall.SIGTERM, syscall.SIGINT)
	//Start  working goroutine
	go func() {
		//Connect to k8s api server
		k8sAPIClientset := k8sClient.ConnectToK8s()

		kafkaConsumer := initConsumer(context.Background(), brockerAddress, topic, source_ID)

		defer kafkaConsumer.Close()

		processingConfig := k8sScenarioConfig{ScenarioNamespace: jobNamespace, ScenarioName: scenarioName}

		for {
			_, kafkaValue := consume(context.Background(), kafkaConsumer)
			var message Payload
			err := json.Unmarshal(kafkaValue, &message)
			if err != nil {
				logger.Error("Kafka message unmarshal failed")
			}
			if message.Source_ID+"_"+message.Sub_source_ID == source_ID {
				processingConfig.s3PackagePath = message.Path
				logger.Info("Source ID match", zap.String("desired source", source_ID), zap.String("current source", message.Source_ID+"_"+message.Sub_source_ID))
				go imageProcessing(CascadeScenatioConfig, k8sAPIClientset, statusServerAddress, processingConfig)
			} else {
				logger.Info("Source ID mismatch", zap.String("desired source", source_ID), zap.String("current source", message.Source_ID+"_"+message.Sub_source_ID))
			}
		}
	}()
	sig := <-cancelChan
	logger.Info("Caught SIGTERM", zap.String("Signal", sig.String()))
	// shutdown other goroutines gracefully
	// close other resources
}
