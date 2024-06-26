package k8sclient

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/randsw/cascadescenariocontroller/api/v1alpha1"
	scenarioconfig "github.com/randsw/cascadescenariocontroller/cascadescenario"
	"github.com/randsw/cascadescenariocontroller/logger"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	kubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clientcmd "k8s.io/client-go/tools/clientcmd"
)

type JobStatus int

const (
	notStarted JobStatus = iota
	Running
	Succeeded
	Failed
)

type S3PackagePath struct {
	StageNum    int
	Path        string
	IsLastStage bool
}

func ConnectToK8s() *kubernetes.Clientset {
	var kubeconfig string

	config, err := rest.InClusterConfig()
	if err != nil {
		// fallback to kubeconfigkubeconfig := filepath.Join("~", ".kube", "config")
		if envvar := os.Getenv("KUBECONFIG"); len(envvar) > 0 {
			kubeconfig = envvar
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			logger.Error("The kubeconfig cannot be loaded", zap.String("err", err.Error()))
			os.Exit(1)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error("Failed to create K8s clientset")
		os.Exit(1)
	}

	return clientset
}

func ConnectTOK8sDynamic() dynamic.Interface {
	var kubeconfig string
	config, err := rest.InClusterConfig()
	if err != nil {
		// fallback to kubeconfigkubeconfig := filepath.Join("~", ".kube", "config")
		if envvar := os.Getenv("KUBECONFIG"); len(envvar) > 0 {
			kubeconfig = envvar
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			logger.Error("The kubeconfig cannot be loaded", zap.String("err", err.Error()))
			os.Exit(1)
		}
	}
	clientset, err := dynamic.NewForConfig(config)
	if err != nil {
		logger.Error("Failed to create dynamic K8s clientset")
		os.Exit(1)
	}

	return clientset
}

func LaunchK8sJob(clientset *kubernetes.Clientset, namespace string, config *scenarioconfig.CascadeScenarios, s3path *S3PackagePath) {
	jobs := clientset.BatchV1().Jobs(namespace)

	labels := map[string]string{"app": "Cascade", "modulename": config.ModuleName}
	// Get Job pod spec
	JobTemplate := config.Template
	// Get Scenario parameters
	ScenarioParameters := config.Configuration

	var podEnv []v1.EnvVar
	// Fill pod env vars with scenario parameters
	for key, value := range ScenarioParameters {
		podEnv = append(podEnv, v1.EnvVar{Name: key, Value: value})
	}
	if s3path.StageNum == 0 {
		podEnv = append(podEnv, v1.EnvVar{Name: "s3path", Value: s3path.Path})
	}
	if s3path.StageNum > 0 {
		split := strings.Split(s3path.Path, ".tgz")
		podEnv = append(podEnv, v1.EnvVar{Name: "s3path", Value: split[0] + "-stage-" + strconv.Itoa(s3path.StageNum) + ".tgz"})
	}
	if s3path.IsLastStage {
		podEnv = append(podEnv, v1.EnvVar{Name: "finalstage", Value: "true"})
	}
	JobTemplate.Spec.Containers[0].Env = podEnv

	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ModuleName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template:                JobTemplate,
			TTLSecondsAfterFinished: config.TTLSecondsAfterFinished,
			BackoffLimit:            config.BackoffLimit,
			ActiveDeadlineSeconds:   config.ActiveDeadlineSeconds,
		},
	}

	_, err := jobs.Create(context.TODO(), jobSpec, metav1.CreateOptions{})
	if err != nil {
		logger.Fatal("Failed to create K8s job.", zap.String("error", err.Error()))
	}

	//print job details
	logger.Info("Created K8s job successfully", zap.String("JobName", config.ModuleName))
}

func GetJobStatus(clientset *kubernetes.Clientset, jobName string, jobNamespace string) (JobStatus, error) {
	job, err := clientset.BatchV1().Jobs(jobNamespace).Get(context.TODO(), jobName, metav1.GetOptions{})
	if err != nil {
		return notStarted, err
	}

	if job.Status.Active == 0 && job.Status.Succeeded == 0 && job.Status.Failed == 0 {
		return notStarted, nil
	}

	if job.Status.Succeeded > 0 {
		logger.Info("Job ran successfully", zap.String("JobName", jobName))
		return Succeeded, nil // Job ran successfully
	}

	if job.Status.Failed > 0 {
		logger.Error("Job ran failed", zap.String("JobName", jobName))
		return Failed, nil // Job ran successfully
	}

	return Running, nil
}

func DeleteSuccessJob(clientset *kubernetes.Clientset, jobName string, jobNamespace string) error {
	background := metav1.DeletePropagationBackground
	err := clientset.BatchV1().Jobs(jobNamespace).Delete(context.TODO(), jobName, metav1.DeleteOptions{PropagationPolicy: &background})
	return err
}

func GetCR(clientDynamic dynamic.Interface, namespace string, name string) (*v1alpha1.CascadeAutoOperator, error) {
	var cascadeAutoOperator = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1aplha1", Resource: "cascadeautooperator"}
	var structured v1alpha1.CascadeAutoOperator
	cascadeAutoOperatorResource, err := clientDynamic.Resource(cascadeAutoOperator).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{}, "status")
	if err != nil {
		logger.Error("Error getting CascadeAutoOperator CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return nil, err
	}
	unstructured_item := cascadeAutoOperatorResource.UnstructuredContent()
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured_item, &structured)
	if err != nil {
		logger.Error("Error while converting to structured CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return nil, err
	}
	return &structured, nil
}

func SetActiveCRStatus(clientDynamic dynamic.Interface, namespace string, name string) error {
	var cascadeAutoOperator = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1aplha1", Resource: "cascadeautooperator"}
	structured, err := GetCR(clientDynamic, namespace, name)
	if err != nil {
		return err
	}
	structured.Status.Active++
	cascadeAutoOperatorResourceNew, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&structured)
	if err != nil {
		logger.Error("Error while converting to unstructured CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return err
	}
	cascadeAutoOperatorResourceNewUnstr := unstructured.Unstructured{Object: cascadeAutoOperatorResourceNew}
	_, err = clientDynamic.Resource(cascadeAutoOperator).Namespace(namespace).UpdateStatus(context.TODO(), &cascadeAutoOperatorResourceNewUnstr, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func SetFailedRemoveActiveStatus(clientDynamic dynamic.Interface, namespace string, name string) error {
	var cascadeAutoOperator = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1aplha1", Resource: "cascadeautooperator"}
	structured, err := GetCR(clientDynamic, namespace, name)
	if err != nil {
		return err
	}
	if structured.Status.Active > 0 {
		structured.Status.Active--
	}
	structured.Status.Failed++
	structured.Status.Result = "Fail"
	cascadeAutoOperatorResourceNew, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&structured)
	if err != nil {
		logger.Error("Error while converting to unstructured CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return err
	}
	cascadeAutoOperatorResourceNewUnstr := unstructured.Unstructured{Object: cascadeAutoOperatorResourceNew}
	_, err = clientDynamic.Resource(cascadeAutoOperator).Namespace(namespace).UpdateStatus(context.TODO(), &cascadeAutoOperatorResourceNewUnstr, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func SetSuccessRemoveActiveStatus(clientDynamic dynamic.Interface, namespace string, name string) error {
	var cascadeAutoOperator = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1aplha1", Resource: "cascadeautooperator"}
	structured, err := GetCR(clientDynamic, namespace, name)
	if err != nil {
		return err
	}
	if structured.Status.Active > 0 {
		structured.Status.Active--
	}
	structured.Status.Succeeded++
	structured.Status.Result = "Success"
	cascadeAutoOperatorResourceNew, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&structured)
	if err != nil {
		logger.Error("Error while converting to unstructured CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return err
	}
	cascadeAutoOperatorResourceNewUnstr := unstructured.Unstructured{Object: cascadeAutoOperatorResourceNew}
	_, err = clientDynamic.Resource(cascadeAutoOperator).Namespace(namespace).UpdateStatus(context.TODO(), &cascadeAutoOperatorResourceNewUnstr, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}
