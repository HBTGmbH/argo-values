package commands

import (
	"sort"
	"testing"

	"argo-values/internal/kubernetes"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type refreshCall struct {
	name      string
	namespace string
}

func fakeRefresh(calls *[]refreshCall) refreshApplicationFunc {
	return func(name, namespace string) error {
		*calls = append(*calls, refreshCall{name: name, namespace: namespace})
		return nil
	}
}

func resourceEvent(kind, name string) kubernetes.Event {
	return kubernetes.Event{
		Obj: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": kind,
				"metadata": map[string]interface{}{
					"name": name,
				},
			},
		},
	}
}

func applicationEvent(envs ...map[string]interface{}) kubernetes.Event {
	envSlice := make([]interface{}, 0, len(envs))
	for _, env := range envs {
		envSlice = append(envSlice, env)
	}
	return kubernetes.Event{
		Obj: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": "Application",
				"metadata": map[string]interface{}{
					"name": "app",
				},
				"spec": map[string]interface{}{
					"source": map[string]interface{}{
						"plugin": map[string]interface{}{
							"env": envSlice,
						},
					},
				},
			},
		},
	}
}

func refreshedApps(calls []refreshCall) []string {
	apps := make([]string, 0, len(calls))
	for _, call := range calls {
		apps = append(apps, call.namespace+"/"+call.name)
	}
	sort.Strings(apps)
	return apps
}

func TestUpdateResourcesRefreshesAllMatchingApplications(t *testing.T) {
	applicationResources := map[string][]string{
		"applications/argocd/app1": {"configmaps/default/cm"},
		"applications/argocd/app2": {"configmaps/default/cm"},
	}

	var calls []refreshCall
	handler := updateResources(false, fakeRefresh(&calls), applicationResources)
	handler(map[string]kubernetes.Event{
		"configmaps/default/cm": resourceEvent("ConfigMap", "cm"),
	})

	got := refreshedApps(calls)
	want := []string{"argocd/app1", "argocd/app2"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected applications %v to be refreshed, got %v", want, got)
	}
}

func TestUpdateResourcesDeduplicatesApplicationRefreshes(t *testing.T) {
	applicationResources := map[string][]string{
		"applications/argocd/app1": {"configmaps/default/cm1", "configmaps/default/cm2"},
	}

	var calls []refreshCall
	handler := updateResources(false, fakeRefresh(&calls), applicationResources)
	handler(map[string]kubernetes.Event{
		"configmaps/default/cm1": resourceEvent("ConfigMap", "cm1"),
		"configmaps/default/cm2": resourceEvent("ConfigMap", "cm2"),
	})

	if len(calls) != 1 {
		t.Fatalf("expected application to be refreshed exactly once, got %d calls: %v", len(calls), calls)
	}
	if calls[0].name != "app1" || calls[0].namespace != "argocd" {
		t.Fatalf("expected argocd/app1 to be refreshed, got %v", calls[0])
	}
}

func TestUpdateResourcesIgnoresUnrelatedApplications(t *testing.T) {
	applicationResources := map[string][]string{
		"applications/argocd/app1": {"configmaps/default/cm1"},
		"applications/argocd/app2": {"configmaps/default/other"},
	}

	var calls []refreshCall
	handler := updateResources(false, fakeRefresh(&calls), applicationResources)
	handler(map[string]kubernetes.Event{
		"configmaps/default/cm1": resourceEvent("ConfigMap", "cm1"),
	})

	got := refreshedApps(calls)
	if len(got) != 1 || got[0] != "argocd/app1" {
		t.Fatalf("expected only argocd/app1 to be refreshed, got %v", got)
	}
}

func TestUpdateResourcesDryRunDoesNotRefresh(t *testing.T) {
	applicationResources := map[string][]string{
		"applications/argocd/app1": {"configmaps/default/cm"},
		"applications/argocd/app2": {"configmaps/default/cm"},
	}

	var calls []refreshCall
	handler := updateResources(true, fakeRefresh(&calls), applicationResources)
	handler(map[string]kubernetes.Event{
		"configmaps/default/cm": resourceEvent("ConfigMap", "cm"),
	})

	if len(calls) != 0 {
		t.Fatalf("expected no refresh calls in dry-run mode, got %v", calls)
	}
}

func TestGetKeysFromObjsUsesDefaultNamespace(t *testing.T) {
	got := getKeysFromObjs("configmaps", []string{"cm"}, "argocd")
	want := []string{"configmaps/argocd/cm"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestGetKeysFromObjsKeepsQualifiedNames(t *testing.T) {
	got := getKeysFromObjs("configmaps", []string{"ns/cm"}, "argocd")
	want := []string{"configmaps/ns/cm"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestGetKeysFromObjsDoesNotMutateInput(t *testing.T) {
	names := []string{"cm", "ns/cm"}
	got := getKeysFromObjs("configmaps", names, "argocd")
	wantInput := []string{"cm", "ns/cm"}
	if len(names) != len(wantInput) || names[0] != wantInput[0] || names[1] != wantInput[1] {
		t.Fatalf("expected input to remain %v, got %v", wantInput, names)
	}
	want := []string{"configmaps/argocd/cm", "configmaps/ns/cm"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestGetApplicationResourcesUsesApplicationNamespace(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Application",
			"metadata": map[string]interface{}{
				"name":      "app",
				"namespace": "argocd",
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"plugin": map[string]interface{}{
						"env": []interface{}{
							map[string]interface{}{"name": "value-configs", "value": "cm"},
						},
					},
				},
			},
		},
	}

	got := getApplicationResources(obj)
	want := []string{"configmaps/argocd/cm"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestGetApplicationResourcesWithoutNamespaceUsesDefault(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind": "Application",
			"metadata": map[string]interface{}{
				"name": "app",
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"plugin": map[string]interface{}{
						"env": []interface{}{
							map[string]interface{}{"name": "value-configs", "value": "cm"},
						},
					},
				},
			},
		},
	}

	got := getApplicationResources(obj)
	want := []string{"configmaps/default/cm"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestUpdateResourcesApplicationEventUpdatesResourceMap(t *testing.T) {
	applicationResources := map[string][]string{}

	var calls []refreshCall
	handler := updateResources(false, fakeRefresh(&calls), applicationResources)
	handler(map[string]kubernetes.Event{
		"applications/argocd/app1": applicationEvent(
			map[string]interface{}{"name": "values-configs", "value": "cm1,cm2"},
		),
	})

	resources, ok := applicationResources["applications/argocd/app1"]
	if !ok {
		t.Fatalf("expected application key to be present in map, got %v", applicationResources)
	}
	want := []string{"configmaps/default/cm1", "configmaps/default/cm2"}
	if len(resources) != len(want) {
		t.Fatalf("expected resources %v, got %v", want, resources)
	}
	for i := range want {
		if resources[i] != want[i] {
			t.Fatalf("expected resources %v, got %v", want, resources)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("expected no refresh calls for application event, got %v", calls)
	}
}
