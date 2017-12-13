package main

import (
	"os"

	"fmt"

	"path"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var whilelist = map[string]bool{
	"pods":        true,
	"deployments": true,
	"replicasets": true,
}

func main() {
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		logrus.Fatal(err)
	}
	contentConfig := rest.ContentConfig{}
	contentConfig.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}
	kubeConfig.ContentConfig = contentConfig
	restClient, err := rest.UnversionedRESTClientFor(kubeConfig)
	if err != nil {
		logrus.Fatal(err)
	}
	clientset := kubernetes.NewForConfigOrDie(kubeConfig)
	out, err := export("default", clientset, restClient, whilelist)
	if err != nil {
		logrus.Fatal(err)
	}
	fmt.Println(out)
}

func export(namespace string, clientset *kubernetes.Clientset, restClient *rest.RESTClient, whitelist map[string]bool) (string, error) {
	result := map[string]interface{}{}
	resourceLists, err := clientset.ServerPreferredNamespacedResources()
	if err != nil {
		return "", err
	}
	for _, resourceList := range resourceLists {
		for _, resource := range resourceList.APIResources {
			if whitelist[resource.Name] {
				if _, ok := result[resource.Name]; ok {
					continue
				}
				list := &unstructured.UnstructuredList{}
				apiPath := "api"
				groupVersion := resourceList.GroupVersion
				if resource.Group != "" && resource.Version != "" {
					groupVersion = path.Join(resource.Group, resource.Version)
				}
				if strings.Contains(groupVersion, "/") {
					apiPath = "apis"
				}
				version := ""
				parts := strings.Split(groupVersion, "/")
				if len(parts) == 1 {
					version = groupVersion
				} else {
					version = parts[1]
				}
				request := restClient.
					Get().
					Prefix(apiPath).
					Prefix(groupVersion).
					Resource(resource.Name).
					Namespace(namespace)
				if err := request.Do().Into(list); err != nil {
					return "", err
				}
				listMap := []map[string]interface{}{}
				for _, item := range list.Items {
					metadata := item.Object["metadata"].(map[string]interface{})
					name := metadata["name"].(string)
					request := restClient.
						Get().
						Prefix(apiPath).
						Prefix(groupVersion).
						Resource(resource.Name).
						Namespace(namespace).
						Name(name).
						Param("export", "true")
					var obj unstructured.Unstructured
					if err := request.Do().Into(&obj); err != nil {
						return "", err
					}
					obj.Object["kind"] = resource.Kind
					obj.Object["apiVersion"] = version
					listMap = append(listMap, obj.Object)
				}
				if len(listMap) > 0 {
					result[resource.Name] = listMap
				}
			}
		}
	}
	out, err := yaml.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
