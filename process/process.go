package process

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	scenarioconfig "github.com/randsw/cascadescenariocontroller/cascadescenario"
	k8sClient "github.com/randsw/cascadescenariocontroller/k8sclient"
	zapLogger "github.com/randsw/cascadescenariocontroller/logger"
	promexporter "github.com/randsw/cascadescenariocontroller/prometheus-exporter"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
	kubernetes "k8s.io/client-go/kubernetes"
)

var CurrProcess int64

const (
	notStarted k8sClient.JobStatus = iota
	Running
	Succeeded
	Failed
)

func ImageProcessing(cascadeScenatioConfig []scenarioconfig.CascadeScenarios, k8sAPIClientset *kubernetes.Clientset, k8sAPIClientDynamic dynamic.Interface,
	k8sProcessingParameters k8sClient.K8sScenarioConfig, GlobalChannel chan map[string]string) {
	stop := make(chan bool)
	// Measure scenarion time
	var wg sync.WaitGroup
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	start_time := time.Now()
	promexporter.CurrentRuns.WithLabelValues(k8sProcessingParameters.ScenarioName, k8sProcessingParameters.Ob, k8sProcessingParameters.SName).Inc()
	s3PkgPath := new(k8sClient.S3TransferPath)
	s3PkgPath.UUID = k8sProcessingParameters.TUUID
	s3PkgPath.OutMinioAddress = k8sProcessingParameters.OutMinioAddress
	// Create cascade run CRD
	err := k8sClient.CreateCascadeRunCRD(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5], k8sProcessingParameters, cascadeScenatioConfig)
	if err != nil {
		zapLogger.Error("Error while creating CascadeRun", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
		return
	}
	wg.Add(1)
	go func(stop chan bool) {
		defer wg.Done()
		for {
			select {
			case <-stop:
				zapLogger.Info("Close CR status update goroutine", zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
				return
			default:
				select {
				case result := <-GlobalChannel:
					zapLogger.Info("Get data from channel", zap.String("Data", string(fmt.Sprintf("%v", result))))
					err := k8sClient.SetCascadeRunStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, result["runname"], result)
					if err != nil {
						// random sleep min-max seconds
						min := 5
						max := 20
						randomTime := r.Intn(max-min) + min
						zapLogger.Error("Error while change CR status. Retrying....", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", result["runname"]), zap.String("TransferUUID", s3PkgPath.UUID))
						time.Sleep(time.Second * time.Duration(randomTime))
						zapLogger.Info("Retry change CR status", zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
						err := k8sClient.SetCascadeRunStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, result["runname"], result)
						if err != nil {
							zapLogger.Error("Error while second attempt to change CR status.", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", result["runname"]), zap.String("TransferUUID", s3PkgPath.UUID))
						}
					}
				default:
					continue
				}
			}
		}
	}(stop)
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := k8sClient.SetActiveCRDStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
		if err != nil {
			// Try Again one time with little sleep period like 100 ms if we get concurent access to CR
			// random sleep min-max seconds
			min := 5
			max := 20
			randomTime := r.Intn(max-min) + min
			zapLogger.Error("Error while change CR status. Retrying....", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
			time.Sleep(time.Second * time.Duration(randomTime))
			zapLogger.Info("Retry change CR status", zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
			err := k8sClient.SetActiveCRDStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
			if err != nil {
				zapLogger.Error("Error while second attempt to change CR status.", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
			}
		}
	}()
	// Increment process count
	zapLogger.Info("Current Processes count", zap.Int64("Running Processes", CurrProcess), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName))
	atomic.AddInt64(&CurrProcess, 1)
	for i, jobConfig := range cascadeScenatioConfig {
		// First stage. Get path from config
		if i == 0 {
			s3PkgPath.Path = k8sProcessingParameters.S3Path
		}
		if i == len(cascadeScenatioConfig)-1 {
			s3PkgPath.IsLastStage = true
		}
		s3PkgPath.StageNum = i
		//Generate random Job name
		jobConfig.ModuleName += "-" + s3PkgPath.UUID[0:5]
		//Start k8s job
		start_time_job := time.Now()
		k8sClient.LaunchK8sJob(k8sAPIClientset, k8sProcessingParameters.ScenarioNamespace, &jobConfig, s3PkgPath, k8sProcessingParameters.ScenarioName)
		start := true
		//Check for job status
		for {
			restart_count := 600
			status, err := k8sClient.GetJobStatus(k8sAPIClientset, k8sProcessingParameters.ScenarioName+"-"+jobConfig.ModuleName, k8sProcessingParameters.ScenarioNamespace)
			if err != nil {
				zapLogger.Error("Get Job status fail", zap.String("JobName", k8sProcessingParameters.ScenarioName+"-"+jobConfig.ModuleName), zap.String("TransferUUID", s3PkgPath.UUID), zap.String("error", err.Error()))
				restart_count--
				if restart_count == 0 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := k8sClient.SetFailedRemoveActiveStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
						if err != nil {
							// random sleep min-max seconds
							min := 5
							max := 20
							randomTime := r.Intn(max-min) + min
							zapLogger.Error("Error while change CR status. Retrying....", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
							time.Sleep(time.Second * time.Duration(randomTime))
							zapLogger.Info("Retry change CR status", zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
							err := k8sClient.SetFailedRemoveActiveStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
							if err != nil {
								zapLogger.Error("Error while second attempt to change CR status.", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
							}
						}
					}()
					// Set final status in cascaderuns CR
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := k8sClient.SetCascadeRunFinalStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5], false, "")
						if err != nil {
							// random sleep min-max seconds
							min := 5
							max := 20
							randomTime := r.Intn(max-min) + min
							zapLogger.Error("Error while change CR status. Retrying....", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5]), zap.String("TransferUUID", s3PkgPath.UUID))
							time.Sleep(time.Second * time.Duration(randomTime))
							zapLogger.Info("Retry change CR status", zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5]), zap.String("TransferUUID", s3PkgPath.UUID))
							err := k8sClient.SetCascadeRunFinalStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5], true, "")
							if err != nil {
								zapLogger.Error("Error while second attempt to change CR status.", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5]), zap.String("TransferUUID", s3PkgPath.UUID))
							}
						}
					}()
					// Wait for all statuses flush to CRD
					zapLogger.Error("Scenario failed. Couldn't get job status", zap.String("JobName", k8sProcessingParameters.ScenarioName+"-"+jobConfig.ModuleName), zap.String("TransferUUID", s3PkgPath.UUID), zap.String("error", err.Error()))
					promexporter.TotalFailedScenario.WithLabelValues(k8sProcessingParameters.ScenarioName, k8sProcessingParameters.Ob, k8sProcessingParameters.SName).Inc()
					promexporter.CurrentRuns.WithLabelValues(k8sProcessingParameters.ScenarioName, k8sProcessingParameters.Ob, k8sProcessingParameters.SName).Dec()
					zapLogger.Info("Closing control channel")
					stop <- true
					wg.Wait()
					atomic.AddInt64(&CurrProcess, -1)
					runtime.Goexit()
				}
			}
			// Job starting
			if status == Running && start {
				zapLogger.Info("Job started ", zap.String("JobName", k8sProcessingParameters.ScenarioName+"-"+jobConfig.ModuleName))
				start = false
			} else if status == Succeeded { // Job finished succesfuly
				// Delete finished Job
				err = k8sClient.DeleteSuccessJob(k8sAPIClientset, k8sProcessingParameters.ScenarioName+"-"+jobConfig.ModuleName, k8sProcessingParameters.ScenarioNamespace)
				if err != nil {
					zapLogger.Error("Failed to delete successfull job", zap.String("JobName", k8sProcessingParameters.ScenarioName+"-"+jobConfig.ModuleName), zap.String("TransferUUID", s3PkgPath.UUID), zap.String("error", err.Error()))
				}
				promexporter.JobDuration.WithLabelValues(k8sProcessingParameters.ScenarioName, jobConfig.ModuleName, k8sProcessingParameters.Ob, k8sProcessingParameters.SName, "success").Observe(time.Since(start_time_job).Seconds())
				break
			} else if status == Failed { // Job failed
				zapLogger.Error("Scenario execution failed", zap.String("Failed Job", k8sProcessingParameters.ScenarioName+"-"+jobConfig.ModuleName), zap.String("TransferUUID", s3PkgPath.UUID), zap.String("Failed scenario", k8sProcessingParameters.ScenarioName))
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := k8sClient.SetFailedRemoveActiveStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
					if err != nil {
						// random sleep min-max seconds
						min := 5
						max := 20
						randomTime := r.Intn(max-min) + min
						zapLogger.Error("Error while change CR status. Retrying....", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
						time.Sleep(time.Second * time.Duration(randomTime))
						zapLogger.Info("Retry change CR status", zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
						err := k8sClient.SetActiveCRDStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
						if err != nil {
							zapLogger.Error("Error while second attempt to change CR status.", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
						}
					}
				}()
				// Set final status in cascaderuns CR
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := k8sClient.SetCascadeRunFinalStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5], false, "")
					if err != nil {
						// random sleep min-max seconds
						min := 5
						max := 20
						randomTime := r.Intn(max-min) + min
						zapLogger.Error("Error while change CR status. Retrying....", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5]), zap.String("TransferUUID", s3PkgPath.UUID))
						time.Sleep(time.Second * time.Duration(randomTime))
						zapLogger.Info("Retry change CR status", zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5]), zap.String("TransferUUID", s3PkgPath.UUID))
						err := k8sClient.SetCascadeRunFinalStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5], true, "")
						if err != nil {
							zapLogger.Error("Error while second attempt to change CR status.", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5]), zap.String("TransferUUID", s3PkgPath.UUID))
						}
					}
				}()
				promexporter.JobDuration.WithLabelValues(k8sProcessingParameters.ScenarioName, jobConfig.ModuleName, k8sProcessingParameters.Ob, k8sProcessingParameters.SName, "fail").Observe(time.Since(start_time_job).Seconds())
				promexporter.TotalFailedScenario.WithLabelValues(k8sProcessingParameters.ScenarioName, k8sProcessingParameters.Ob, k8sProcessingParameters.SName).Inc()
				promexporter.CurrentRuns.WithLabelValues(k8sProcessingParameters.ScenarioName, k8sProcessingParameters.Ob, k8sProcessingParameters.SName).Dec()
				zapLogger.Info("Closing control channel")
				stop <- true
				wg.Wait()
				atomic.AddInt64(&CurrProcess, -1)
				runtime.Goexit()
			}
		}
	}
	zapLogger.Info("Scenario execution finished successfully", zap.String("Scenario Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := k8sClient.SetSuccessRemoveActiveStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
		if err != nil {
			// random sleep min-max seconds
			min := 5
			max := 20
			randomTime := r.Intn(max-min) + min
			zapLogger.Error("Error while change CR status. Retrying....", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
			time.Sleep(time.Second * time.Duration(randomTime))
			zapLogger.Info("Retry change CR status", zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
			err := k8sClient.SetActiveCRDStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName)
			if err != nil {
				zapLogger.Error("Error while second attempt to change CR status.", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName), zap.String("TransferUUID", s3PkgPath.UUID))
			}
		}
	}()
	// Set final status in cascaderuns CR
	wg.Add(1)
	go func() {
		resultAddress := s3PkgPath.OutMinioAddress + k8sProcessingParameters.ScenarioName + "/" + s3PkgPath.UUID + "/" + s3PkgPath.UUID + "-final.zip"
		defer wg.Done()
		err := k8sClient.SetCascadeRunFinalStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5], true, resultAddress)
		if err != nil {
			// random sleep min-max seconds
			min := 5
			max := 20
			randomTime := r.Intn(max-min) + min
			zapLogger.Error("Error while change CR status. Retrying....", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5]), zap.String("TransferUUID", s3PkgPath.UUID))
			time.Sleep(time.Second * time.Duration(randomTime))
			zapLogger.Info("Retry change CR status", zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5]), zap.String("TransferUUID", s3PkgPath.UUID))
			err := k8sClient.SetCascadeRunFinalStatus(k8sAPIClientDynamic, k8sProcessingParameters.ScenarioNamespace, k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5], true, resultAddress)
			if err != nil {
				zapLogger.Error("Error while second attempt to change CR status.", zap.Error(err), zap.String("Namespace", k8sProcessingParameters.ScenarioNamespace), zap.String("Name", k8sProcessingParameters.ScenarioName+"-"+s3PkgPath.UUID[0:5]), zap.String("TransferUUID", s3PkgPath.UUID))
			}
		}
	}()
	// Measure and export scenario execution time
	promexporter.ScenarioDuration.WithLabelValues(k8sProcessingParameters.ScenarioName, k8sProcessingParameters.Ob, k8sProcessingParameters.SName).Observe(time.Since(start_time).Seconds())
	promexporter.TotalSucceedScenario.WithLabelValues(k8sProcessingParameters.ScenarioName, k8sProcessingParameters.Ob, k8sProcessingParameters.SName).Inc()
	promexporter.CurrentRuns.WithLabelValues(k8sProcessingParameters.ScenarioName, k8sProcessingParameters.Ob, k8sProcessingParameters.SName).Dec()
	zapLogger.Info("Closing control channel")
	stop <- true
	wg.Wait()
	atomic.AddInt64(&CurrProcess, -1)
}
