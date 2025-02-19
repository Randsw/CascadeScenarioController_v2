package k8sclient

import (
	"context"
	"fmt"
	"os"
	"strconv"

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

type K8sScenarioConfig struct {
	ScenarioNamespace string
	ScenarioName      string
	S3Path            string
	TUUID             string
	OutMinioAddress   string
	SName             string
	Ob                string
}

type S3TransferPath struct {
	StageNum        int
	Path            string
	IsLastStage     bool
	UUID            string
	OutMinioAddress string
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

func ConnectTOK8sDinamic() dynamic.Interface {
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

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		logger.Error("Failed to create dynamic K8s clientset")
		os.Exit(1)
	}
	return dynClient
}

func LaunchK8sJob(clientset *kubernetes.Clientset, namespace string, config *scenarioconfig.CascadeScenarios, s3path *S3TransferPath, scenarioName string) {
	minio_test := s3path.OutMinioAddress + scenarioName + "/" + s3path.UUID + "/"
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
		//split := strings.Split(s3path.Path, ".zip")
		podEnv = append(podEnv, v1.EnvVar{Name: "s3path", Value: minio_test + s3path.UUID + "-stage-" + strconv.Itoa(s3path.StageNum) + ".zip"})
	}
	if s3path.IsLastStage {
		podEnv = append(podEnv, v1.EnvVar{Name: "finalstage", Value: "true"})
	}
	podEnv = append(podEnv, v1.EnvVar{Name: "NAMESPACE", ValueFrom: &v1.EnvVarSource{
		FieldRef: &v1.ObjectFieldSelector{
			FieldPath: "metadata.namespace",
		},
	},
	})
	podEnv = append(podEnv, v1.EnvVar{Name: "NAMESPACE", ValueFrom: &v1.EnvVarSource{
		FieldRef: &v1.ObjectFieldSelector{
			FieldPath: "metadata.namespace",
		},
	},
	})
	podEnv = append(podEnv, v1.EnvVar{Name: "SCENARIONAME", Value: scenarioName})

	JobTemplate.Spec.Containers[0].Env = podEnv

	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scenarioName + "-" + config.ModuleName,
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
	var cascadeAutoOperator = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1alpha1", Resource: "cascadeautooperators"}
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

func GetCRRun(clientDynamic dynamic.Interface, namespace string, name string) (*v1alpha1.CascadeRun, error) {
	var cascadeRun = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1alpha1", Resource: "cascaderuns"}
	var structured v1alpha1.CascadeRun
	cascadeRunResource, err := clientDynamic.Resource(cascadeRun).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{}, "status")
	if err != nil {
		logger.Error("Error getting CascadeRun CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return nil, err
	}
	unstructured_item := cascadeRunResource.UnstructuredContent()
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructured_item, &structured)
	if err != nil {
		logger.Error("Error while converting to structured CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return nil, err
	}
	return &structured, nil
}

func SetActiveCRDStatus(clientDynamic dynamic.Interface, namespace string, name string) error {
	var cascadeAutoOperator = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1alpha1", Resource: "cascadeautooperators"}
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
	var cascadeAutoOperator = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1alpha1", Resource: "cascadeautooperators"}
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
	var cascadeAutoOperator = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1alpha1", Resource: "cascadeautooperators"}
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

func CreateCascadeRunCRD(clientDynamic dynamic.Interface, namespace string, name string, k8sProcessingParameters K8sScenarioConfig, cascadeScenatioConfig []scenarioconfig.CascadeScenarios) error {
	// Create modules Name list
	var modules []string
	for _, module := range cascadeScenatioConfig {
		modules = append(modules, module.ModuleName)
	}
	crd := &v1alpha1.CascadeRun{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CascadeRun",
			APIVersion: "cascade.cascade.net/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.CascadeRunSpec{
			Ob:           k8sProcessingParameters.Ob,
			Src:          k8sProcessingParameters.SName,
			PID:          k8sProcessingParameters.TUUID,
			ScenarioName: k8sProcessingParameters.ScenarioName,
			//Modules name
			Modules: modules,
		},
	}
	gvr := schema.GroupVersionResource{
		Group:    "cascade.cascade.net",
		Version:  "v1alpha1",
		Resource: "cascaderuns",
	}
	CascadecascadeRun, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&crd)
	if err != nil {
		logger.Error("Error while converting CascadeRun to unstructured CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return err
	}
	_, err = clientDynamic.Resource(gvr).Namespace(namespace).Create(context.TODO(), &unstructured.Unstructured{Object: CascadecascadeRun}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func SetCascadeRunStatus(clientDynamic dynamic.Interface, namespace string, name string, status map[string]string) error {
	var cascadeRun = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1alpha1", Resource: "cascaderuns"}
	structured, err := GetCRRun(clientDynamic, namespace, name)
	if err != nil {
		return err
	}
	for k, v := range status {
		if k != "runname" {
			str := fmt.Sprintf("%s: %s\\n", k, v)
			structured.Status.Result = append(structured.Status.Result, str)
		}
	}
	cascadeRunResourceNew, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&structured)
	if err != nil {
		logger.Error("Error while converting to unstructured CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return err
	}
	cascadeRunResourceNewUnstr := unstructured.Unstructured{Object: cascadeRunResourceNew}
	_, err = clientDynamic.Resource(cascadeRun).Namespace(namespace).UpdateStatus(context.TODO(), &cascadeRunResourceNewUnstr, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func SetCascadeRunFinalStatus(clientDynamic dynamic.Interface, namespace string, name string, isSuccess bool, resultAddress string) error {
	var cascadeRun = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1alpha1", Resource: "cascaderuns"}
	structured, err := GetCRRun(clientDynamic, namespace, name)
	if err != nil {
		return err
	}
	if isSuccess {
		structured.Status.Info = resultAddress
	} else {
		structured.Status.Info = "Failed"
	}
	cascadeRunResourceNew, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&structured)
	if err != nil {
		logger.Error("Error while converting to unstructured CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return err
	}
	cascadeRunResourceNewUnstr := unstructured.Unstructured{Object: cascadeRunResourceNew}
	_, err = clientDynamic.Resource(cascadeRun).Namespace(namespace).UpdateStatus(context.TODO(), &cascadeRunResourceNewUnstr, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func WatchDeletetionTimeStampCRD(clientDynamic dynamic.Interface, namespace string, name string) (bool, error) {
	structured, err := GetCR(clientDynamic, namespace, name)
	if err != nil {
		return false, err
	}
	if structured.DeletionTimestamp != nil {
		return true, nil
	} else {
		return false, nil
	}
}

func DeleteFinalizerCRD(clientDynamic dynamic.Interface, namespace string, name string, finalizer string) error {
	var cascadeAutoOperator = schema.GroupVersionResource{Group: "cascade.cascade.net", Version: "v1alpha1", Resource: "cascadeautooperators"}
	structured, err := GetCR(clientDynamic, namespace, name)
	if err != nil {
		return err
	}
	f := []string{}
	index := 0
	for i := 0; i < len(structured.Finalizers); i++ {
		if structured.Finalizers[i] == finalizer {
			logger.Info("Found desired finalizer", zap.String("Finalizer", structured.Finalizers[i]))
			continue
		}
		f[index] = structured.Finalizers[i]
		index++
	}
	structured.Finalizers = f
	logger.Info("Delete finalizer in CRD", zap.String("Namespace", namespace), zap.String("Name", name))
	cascadeRunResourceNew, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&structured)
	if err != nil {
		logger.Error("Error while converting to unstructured CR", zap.String("Namespace", namespace), zap.String("Name", name))
		return err
	}
	cascadeAutoOperatorResourceNewUnstr := unstructured.Unstructured{Object: cascadeRunResourceNew}
	_, err = clientDynamic.Resource(cascadeAutoOperator).Namespace(namespace).Update(context.TODO(), &cascadeAutoOperatorResourceNewUnstr, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}
