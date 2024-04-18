package main

import (
	"context"
	"math/rand"
	"runtime"
	"strings"
	"time"

	scenarioconfig "github.com/randsw/cascadescenariocontroller/cascadescenario"
	k8sClient "github.com/randsw/cascadescenariocontroller/k8sclient"
	zapLogger "github.com/randsw/cascadescenariocontroller/logger"
	promexporter "github.com/randsw/cascadescenariocontroller/prometheus-exporter"
	webhook "github.com/randsw/cascadescenariocontroller/webhook"
	kafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	kubernetes "k8s.io/client-go/kubernetes"
)

func RandStringBytesRmndr(n int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	const letterBytes = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[r.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}

func initConsumer(ctx context.Context, brokerAddress string, topic string, source_ID string) *kafka.Reader {
	// initialize a new reader with the brokers and topic
	// the groupID identifies the consumer and prevents
	// it from receiving duplicate messages
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{brokerAddress},
		Topic:   topic,
		GroupID: source_ID,
		// assign the logger to the reader
		//Logger: zapLogger.Zaplogger,
		Logger:      kafka.LoggerFunc(zapLogger.KafkaInfo),
		ErrorLogger: kafka.LoggerFunc(zapLogger.KafkaError),
	})

	return r
}

func consume(ctx context.Context, r *kafka.Reader) ([]byte, []byte) {
	msg, err := r.ReadMessage(ctx)
	if err != nil {
		zapLogger.Error("could not read message ", zap.String("error", err.Error()))
	}
	return msg.Key, msg.Value
}

func imageProcessing(cascadeScenatioConfig []scenarioconfig.CascadeScenarios, k8sAPIClientset *kubernetes.Clientset, k8sAPIClientDynamic dynamic.Interface,
	statusServerAddress string, k8sProcessingParameters k8sScenarioConfig) {
	// Measure scenarion time
	start_time := time.Now()

	s3PkgPath := new(k8sClient.S3PackagePath)
	randString := RandStringBytesRmndr(5)
	err := k8sClient.SetActiveCRStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
	if err != nil {
		zapLogger.Error("Error while change CR status", zap.Error(err))
	}
	for i, jobConfig := range cascadeScenatioConfig {
		// First stage. Get path from config
		if i == 0 {
			s3PkgPath.Path = k8sProcessingParameters.s3PackagePath
		}
		if i == len(cascadeScenatioConfig)-1 {
			s3PkgPath.IsLastStage = true
		}
		s3PkgPath.StageNum = i
		//Generate random Job name
		jobConfig.ModuleName += "-" + randString
		//Start k8s job
		start_time_job := time.Now()
		k8sClient.LaunchK8sJob(k8sAPIClientset, k8sProcessingParameters.ScenarioNamespace, &jobConfig, s3PkgPath)
		start := true
		//Check for job status
		for {
			status, err := k8sClient.GetJobStatus(k8sAPIClientset, jobConfig.ModuleName, k8sProcessingParameters.ScenarioNamespace)
			if err != nil {
				zapLogger.Error("Get Job status fail", zap.String("JobName", jobConfig.ModuleName), zap.String("error", err.Error()))
			}
			// Job starting
			if status == Running && start {
				zapLogger.Info("Job started ", zap.String("JobName", jobConfig.ModuleName))
				start = false
			} else if status == Succeeded { // Job finished succesfuly
				statusCode, err := webhook.SendWebHook("Module "+jobConfig.ModuleName+" in scenario "+k8sProcessingParameters.ScenarioName+" finished successfully", statusServerAddress)
				if err != nil {
					zapLogger.Error("Webhook failed", zap.String("error", err.Error()))
				}
				if statusCode != "200" {
					zapLogger.Error("Webhook return fail code", zap.String("error", err.Error()))
				}
				// Delete finished Job
				err = k8sClient.DeleteSuccessJob(k8sAPIClientset, jobConfig.ModuleName, k8sProcessingParameters.ScenarioNamespace)
				if err != nil {
					zapLogger.Error("Failed to delete successfull job", zap.String("JobName", jobConfig.ModuleName), zap.String("error", err.Error()))
				}
				promexporter.JobDuration.WithLabelValues(jobConfig.ModuleName).Observe(time.Since(start_time_job).Seconds())
				break
			} else if status == Failed { // Job failed
				statusCode, err := webhook.SendWebHook("Module "+jobConfig.ModuleName+" in scenario "+k8sProcessingParameters.ScenarioName+" failed", statusServerAddress)
				if err != nil {
					zapLogger.Error("Webhook failed", zap.String("error", err.Error()))
				}
				if statusCode != "200" {
					zapLogger.Error("Webhook return fail code", zap.String("error", err.Error()))
				}
				zapLogger.Error("Scenario execution failed", zap.String("Failed Job", jobConfig.ModuleName), zap.String("Failed scenario", k8sProcessingParameters.ScenarioName))
				err = k8sClient.SetFailedRemoveActiveStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
				if err != nil {
					zapLogger.Error("Error while change CR status", zap.Error(err))
				}
				promexporter.JobDuration.WithLabelValues(jobConfig.ModuleName).Observe(time.Since(start_time_job).Seconds())
				runtime.Goexit()
			}
		}
	}
	zapLogger.Info("Scenario execution finished successfully", zap.String("Scenario Name", k8sProcessingParameters.ScenarioName))
	err = k8sClient.SetSuccessRemoveActiveStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
	if err != nil {
		zapLogger.Error("Error while change CR status", zap.Error(err))
	}
	split := strings.Split(s3PkgPath.Path, ".tgz")
	resultS3path := split[0] + "-final" + ".tgz"
	// Measure and export scenario execution time
	promexporter.ScenarioDuration.WithLabelValues("scenario").Observe(time.Since(start_time).Seconds())
	statusCode, err := webhook.SendWebHook("Scenario "+k8sProcessingParameters.ScenarioName+" completed successfully. Package address - "+resultS3path, statusServerAddress)
	if err != nil {
		zapLogger.Error("Webhook failed", zap.String("error", err.Error()))
	}
	if statusCode != "200" {
		zapLogger.Error("Webhook return fail code", zap.String("error", err.Error()))
	}
}
