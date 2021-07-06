package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	userDefinedUserString string = "OPSH"
	defaultOutputDir      string = "default"
)

func extract(unknown interface{}) runtime.Object {

	/*
		given that there are many fields which we will not want, it's easier to create an initial, empty, default
		instance of each type, then copy over select few fields - then return as a runtime.Object for further processing
	*/

	switch v := unknown.(type) {
	case appsv1.Deployment:
		newP := appsv1.Deployment{}
		newP.TypeMeta = v.TypeMeta
		newP.ObjectMeta.Labels = v.ObjectMeta.Labels
		newP.ObjectMeta.Name = v.ObjectMeta.Name
		newP.ObjectMeta.Namespace = v.ObjectMeta.Namespace
		newP.Spec = v.Spec
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

	case rbacv1.Role:
		newP := rbacv1.Role{}
		newP.TypeMeta = v.TypeMeta
		newP.ObjectMeta.Labels = v.ObjectMeta.Labels
		newP.ObjectMeta.Name = v.ObjectMeta.Name
		newP.ObjectMeta.Namespace = v.ObjectMeta.Namespace
		newP.Rules = v.Rules
		return newP.DeepCopyObject()

	case rbacv1.ClusterRoleBinding:
		newP := rbacv1.ClusterRoleBinding{}
		newP.TypeMeta = v.TypeMeta
		newP.ObjectMeta.Labels = v.ObjectMeta.Labels
		newP.ObjectMeta.Name = v.ObjectMeta.Name
		newP.ObjectMeta.Namespace = v.ObjectMeta.Namespace
		newP.RoleRef = v.RoleRef
		newP.Subjects = v.Subjects
		return newP.DeepCopyObject()

	case *rbacv1.ClusterRole:
		newP := rbacv1.ClusterRole{}
		newP.TypeMeta = v.TypeMeta
		newP.ObjectMeta.Labels = v.ObjectMeta.Labels
		newP.ObjectMeta.Name = v.ObjectMeta.Name
		newP.Rules = v.Rules
		return newP.DeepCopyObject()

	}
	return nil
}

type fileWriter struct {
	rootDir string
	buffer  bytes.Buffer
}

// implement Writer interface
func (f *fileWriter) Write(p []byte) (n int, err error) {
	return f.buffer.Write(p)
}

func (f *fileWriter) flush(namespace, name, resourceType string) error {
	/*
		write the byte stream to a file, in the following format:
		rootDir / namespaces / namespaceName / resourceType / filename
		rootDir / non_namespaced / resourceType / filename
	*/
	path := f.rootDir

	if namespace != "" {
		path = f.rootDir + "/namespaces/" + namespace + "/" + resourceType
	} else {
		path = f.rootDir + "/non_namespaced/" + resourceType
	}
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path+"/"+name, f.buffer.Bytes(), os.ModePerm)

}

func newFileWriter(rootPath string) *fileWriter {
	return &fileWriter{
		rootDir: rootPath,
		buffer:  bytes.Buffer{},
	}
}

func addTypeInformationToObject(obj runtime.Object) error {
	// without this, the api will return objects without this information

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

func toYaml(c runtime.Object, w *fileWriter) {
	addTypeInformationToObject(c)
	s := json.NewYAMLSerializer(json.DefaultMetaFactory, scheme.Scheme, scheme.Scheme)
	err := s.Encode(c, w)
	if err != nil {
		log.Fatal(err)
	}

}

func isUserDefined(s, lookFor string) bool {
	return strings.Contains(s, lookFor)
}

func containsUserDefined(subjects []rbacv1.Subject, lookFor string) bool {
	for _, subject := range subjects {
		if isUserDefined(subject.Name, lookFor) {
			return true
		}
	}
	return false
}

func main() {

	var kubeconfig *string
	var outputDir *string
	var roleRefString *string

	outputDir = flag.String("outdir", defaultOutputDir, "absolute path to the directory to write the yaml files into")
	roleRefString = flag.String("rolestring", userDefinedUserString, "common string used in user-defined role refs: for example, OPSH, or RES-DEV")

	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatal(err)
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	// go through our list of types, and simply grab all we can from the cluster
	deployments, err := clientset.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	for _, deployment := range deployments.Items {
		w := newFileWriter(*outputDir)
		toYaml(extract(deployment), w)
		w.flush(deployment.ObjectMeta.Namespace, deployment.ObjectMeta.Name, "deployment")
	}

	/*
		Most roles and roles bindings within the cluster are either default, or controlled by operators. In order to only extract those which are created for user access
		we need to go through the list of bindings, and only extract those that have a roleRef (membership) that is a user / group that we care about - for example:

		RES-DEV-OPSH-DEVELOPER-FDS_TADPOLE

		Need to work using bindings as the Roles themselves hold no reference to the binding objects
	*/

	bindings, err := clientset.RbacV1().RoleBindings("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	userDefinedBindings := []rbacv1.RoleBinding{}

	for _, binding := range bindings.Items {
		subjects := binding.Subjects
		if containsUserDefined(subjects, *roleRefString) {
			userDefinedBindings = append(userDefinedBindings, binding)
		}
	}

	for _, binding := range userDefinedBindings {
		w := newFileWriter(*outputDir)
		toYaml(extract(binding), w)
		w.flush(binding.ObjectMeta.Namespace, binding.ObjectMeta.Name, "binding")
		opts := metav1.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("metadata.name", binding.RoleRef.Name).String(),
		}
		roles, _ := clientset.RbacV1().Roles(binding.ObjectMeta.Namespace).List(context.TODO(), opts)
		for _, role := range roles.Items {
			w := newFileWriter(*outputDir)
			toYaml(extract(role), w)
			w.flush(role.ObjectMeta.Namespace, role.ObjectMeta.Name, "role")

		}

	}

	// repeat for cluster bindings
	clusterBindings, err := clientset.RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	userDefinedClusterBindings := []rbacv1.ClusterRoleBinding{}

	for _, binding := range clusterBindings.Items {
		subjects := binding.Subjects
		if containsUserDefined(subjects, *roleRefString) {
			userDefinedClusterBindings = append(userDefinedClusterBindings, binding)
		}
	}

	for _, binding := range userDefinedClusterBindings {
		w := newFileWriter(*outputDir)
		toYaml(extract(binding), w)
		w.flush(binding.ObjectMeta.Namespace, binding.ObjectMeta.Name, "clusterbinding")
		role, err := clientset.RbacV1().ClusterRoles().Get(context.TODO(), binding.RoleRef.Name, metav1.GetOptions{})
		if err != nil {
			log.Fatal(err)
		}
		w = newFileWriter(*outputDir)
		toYaml(extract(role), w)
		w.flush(role.ObjectMeta.Namespace, role.ObjectMeta.Name, "clusterrole")

	}

}
