package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/kubectl/pkg/scheme"
)

func extract(unknown interface{}) runtime.Object {

	switch v := unknown.(type) {
	case v1.Pod:
		newP := v1.Pod{}
		newP.TypeMeta = v.TypeMeta
		newP.ObjectMeta.Labels = v.ObjectMeta.Labels
		newP.ObjectMeta.Name = v.ObjectMeta.Name
		newP.ObjectMeta.Namespace = v.ObjectMeta.Namespace
		newP.Spec = v.Spec
		return newP.DeepCopyObject()
	case v1.Secret:
		newP := v1.Secret{}
		newP.TypeMeta = v.TypeMeta
		newP.ObjectMeta.Labels = v.ObjectMeta.Labels
		newP.ObjectMeta.Name = v.ObjectMeta.Name
		newP.ObjectMeta.Namespace = v.ObjectMeta.Namespace
		newP.Data = v.Data
		newP.Type = v.Type
		return newP.DeepCopyObject()
	case rbacv1.RoleBinding:
		newP := rbacv1.RoleBinding{}
		newP.TypeMeta = v.TypeMeta
		newP.ObjectMeta.Labels = v.ObjectMeta.Labels
		newP.ObjectMeta.Name = v.ObjectMeta.Name
		newP.ObjectMeta.Namespace = v.ObjectMeta.Namespace
		newP.RoleRef = v.RoleRef
		newP.Subjects = v.Subjects
		return newP.DeepCopyObject()
	}
	return nil
}

func addTypeInformationToObject(obj runtime.Object) error {
	gvks, _, err := scheme.Scheme.ObjectKinds(obj)
	if err != nil {
		return fmt.Errorf("missing apiVersion or kind and cannot assign it; %w", err)
	}

	for _, gvk := range gvks {
		if len(gvk.Kind) == 0 {
			continue
		}
		if len(gvk.Version) == 0 || gvk.Version == runtime.APIVersionInternal {
			continue
		}
		obj.GetObjectKind().SetGroupVersionKind(gvk)
		break
	}

	return nil
}

func toYaml(c runtime.Object, w io.Writer) {
	addTypeInformationToObject(c)
	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme)
	err := s.Encode(c, w)
	if err != nil {
		panic(err)
	}
}

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, pod := range pods.Items {
		toYaml(extract(pod), os.Stdout)
	}

	secrets, err := clientset.CoreV1().Secrets("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, secret := range secrets.Items {
		toYaml(extract(secret), os.Stdout)

	}

	bindings, err := clientset.RbacV1().RoleBindings("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, secret := range bindings.Items {
		toYaml(extract(secret), os.Stdout)

	}

}
