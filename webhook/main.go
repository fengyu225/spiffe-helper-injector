package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	tlsCertPath = "/etc/webhook/certs/svid.pem"
	tlsKeyPath  = "/etc/webhook/certs/svid_key.pem"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

type WebhookServer struct {
	server     *http.Server
	kubeClient *kubernetes.Clientset
}

func addSpiffeVolumes(pod *corev1.Pod) (patches []map[string]interface{}) {
	requiredVolumes := []map[string]interface{}{
		{
			"name": "spire-agent-socket",
			"hostPath": map[string]interface{}{
				"path": "/run/spire/agent-sockets",
				"type": "Directory",
			},
		},
		{
			"name": "spiffe-helper-config",
			"configMap": map[string]interface{}{
				"name": "webhook-spiffe-helper-config",
			},
		},
		{
			"name":     "spiffe-certs",
			"emptyDir": map[string]interface{}{},
		},
	}

	if pod.Spec.Volumes == nil {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/volumes",
			"value": []interface{}{},
		})
	}

	existingVolumes := make(map[string]bool)
	for _, volume := range pod.Spec.Volumes {
		existingVolumes[volume.Name] = true
	}

	for _, volume := range requiredVolumes {
		volumeName := volume["name"].(string)
		if !existingVolumes[volumeName] {
			patches = append(patches, map[string]interface{}{
				"op":    "add",
				"path":  "/spec/volumes/-",
				"value": volume,
			})
		}
	}

	return patches
}

func addSpiffeInitContainer(pod *corev1.Pod) (patches []map[string]interface{}) {
	spiffeInitContainer := map[string]interface{}{
		"name":            "spiffe-helper-init",
		"image":           "docker.io/fengyu225/spiffe-helper:v0.0.1",
		"imagePullPolicy": "Always",
		"args": []string{
			"-config",
			"/etc/spiffe-helper/helper.conf",
			"-daemon-mode=false",
		},
		"volumeMounts": []map[string]interface{}{
			{
				"name":      "spiffe-helper-config",
				"mountPath": "/etc/spiffe-helper",
			},
			{
				"name":      "spire-agent-socket",
				"mountPath": "/run/spire/agent-sockets",
			},
			{
				"name":      "spiffe-certs",
				"mountPath": "/run/spiffe/certs",
			},
		},
	}

	if pod.Spec.InitContainers == nil {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/initContainers",
			"value": []interface{}{},
		})
	}

	hasSpiffeInit := false
	for _, container := range pod.Spec.InitContainers {
		if container.Name == "spiffe-helper-init" {
			hasSpiffeInit = true
			break
		}
	}

	if !hasSpiffeInit {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/initContainers/-",
			"value": spiffeInitContainer,
		})
	}

	return patches
}

func addSpiffeSidecar(pod *corev1.Pod) (patches []map[string]interface{}) {
	spiffeSidecar := map[string]interface{}{
		"name":            "spiffe-helper",
		"image":           "docker.io/fengyu225/spiffe-helper:v0.0.1",
		"imagePullPolicy": "Always",
		"args": []string{
			"-config",
			"/etc/spiffe-helper/helper.conf",
		},
		"volumeMounts": []map[string]interface{}{
			{
				"name":      "spiffe-helper-config",
				"mountPath": "/etc/spiffe-helper",
			},
			{
				"name":      "spire-agent-socket",
				"mountPath": "/run/spire/agent-sockets",
			},
			{
				"name":      "spiffe-certs",
				"mountPath": "/run/spiffe/certs",
			},
		},
	}

	hasSpiffeSidecar := false
	for _, container := range pod.Spec.Containers {
		if container.Name == "spiffe-helper" {
			hasSpiffeSidecar = true
			break
		}
	}

	if !hasSpiffeSidecar {
		patches = append(patches, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/containers/-",
			"value": spiffeSidecar,
		})
	}

	return patches
}

func createSpiffeHelperConfigMap(namespace string, client *kubernetes.Clientset) error {
	_, err := client.CoreV1().ConfigMaps(namespace).Get(context.Background(), "webhook-spiffe-helper-config", metav1.GetOptions{})
	if err == nil {
		log.Printf("configmap already exists")
		return nil
	}

	configMapData := map[string]string{
		"helper.conf": `agent_address = "/run/spire/agent-sockets/socket"
cert_dir = "/run/spiffe/certs"
svid_file_name = "svid.pem"
svid_key_file_name = "svid_key.pem"
svid_bundle_file_name = "svid_bundle.pem"`,
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-spiffe-helper-config",
		},
		Data: configMapData,
	}

	_, err = client.CoreV1().ConfigMaps(namespace).Create(context.Background(), configMap, metav1.CreateOptions{})
	return err
}

func (whsvr *WebhookServer) mutate(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	req := ar.Request
	log.Printf("Handling admission request for %s/%s", req.Namespace, req.Name)

	pod := &corev1.Pod{}
	if err := json.Unmarshal(req.Object.Raw, pod); err != nil {
		log.Printf("Could not unmarshal raw object: %v", err)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	if val, ok := pod.Labels["spiffe.io/spire-managed-identity"]; !ok || val != "true" {
		log.Printf("Pod %s/%s doesn't have required label, skipping", req.Namespace, req.Name)
		return &admissionv1.AdmissionResponse{
			Allowed: true,
		}
	}

	if err := createSpiffeHelperConfigMap(req.Namespace, whsvr.kubeClient); err != nil {
		log.Printf("Warning: Failed to create ConfigMap in namespace %s: %v", req.Namespace, err)
	}

	var patches []map[string]interface{}

	patches = append(patches, addSpiffeVolumes(pod)...)

	patches = append(patches, addSpiffeInitContainer(pod)...)

	patches = append(patches, addSpiffeSidecar(pod)...)

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	log.Printf("Generated patches for pod %s/%s: %s", req.Namespace, req.Name, string(patchBytes))

	return &admissionv1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admissionv1.PatchType {
			pt := admissionv1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

func (whsvr *WebhookServer) serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		log.Printf("Content-Type=%s, want application/json", contentType)
		http.Error(w, "invalid Content-Type, want application/json", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *admissionv1.AdmissionResponse
	ar := admissionv1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		log.Printf("Can't decode body: %v", err)
		admissionResponse = &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		admissionResponse = whsvr.mutate(&ar)
	}

	response := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
	}
	if admissionResponse != nil {
		response.Response = admissionResponse
		if ar.Request != nil {
			response.Response.UID = ar.Request.UID
		}
	}

	resp, err := json.Marshal(response)
	if err != nil {
		log.Printf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(resp); err != nil {
		log.Printf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("Initializing webhook server, writing to stdout")
}

func main() {
	log.Printf("Starting webhook server initialization")

	log.Printf("Creating in-cluster config")
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to get in-cluster config: %v", err)
	}

	log.Printf("Creating Kubernetes clientset")
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	log.Printf("Creating webhook server")
	whsvr := &WebhookServer{
		server: &http.Server{
			Addr: ":8443",
		},
		kubeClient: clientset,
	}

	log.Printf("Setting up HTTP handlers")
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.serve)
	whsvr.server.Handler = mux

	log.Printf("Starting webhook server on :8443")
	go func() {
		log.Printf("About to start ListenAndServeTLS")
		if err := whsvr.server.ListenAndServeTLS(tlsCertPath, tlsKeyPath); err != nil {
			log.Printf("Failed to listen and serve: %v", err)
			os.Exit(1)
		}
	}()

	log.Printf("Webhook server started, waiting for shutdown signal")

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	log.Printf("Received shutdown signal, gracefully shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := whsvr.server.Shutdown(ctx); err != nil {
		log.Printf("Failed to gracefully shutdown: %v", err)
	}
	log.Printf("Server shutdown complete")
}
