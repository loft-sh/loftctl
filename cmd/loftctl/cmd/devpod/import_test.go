package devpod

import (
	"context"
	agentloftclient "github.com/loft-sh/agentapi/v3/pkg/client/loft/clientset_generated/clientset"
	managementv1 "github.com/loft-sh/api/v3/pkg/apis/management/v1"
	loftclient "github.com/loft-sh/api/v3/pkg/client/clientset_generated/clientset"
	v1 "github.com/loft-sh/api/v3/pkg/client/clientset_generated/clientset/typed/management/v1"
	"github.com/pkg/errors"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func TestImportCmd_fetchWorkspace(t *testing.T) {
	ctx := context.Background()
	projectName := "testProject"
	workspaceUID := "testUID"

	tests := []struct {
		name           string
		mockClient     MockInterface
		expectedError  string
		expectedResult *managementv1.DevPodWorkspaceInstance
	}{
		{
			name: "fetchWorkspace returns one workspace",
			mockClient: MockInterface{
				FakeLoft: &FakeLoft{
					DevPodWorkspaceInstanceList: &managementv1.DevPodWorkspaceInstanceList{
						Items: []managementv1.DevPodWorkspaceInstance{
							{ObjectMeta: metav1.ObjectMeta{Name: "workspace1"}},
						},
					},
				},
			},
			expectedResult: &managementv1.DevPodWorkspaceInstance{ObjectMeta: metav1.ObjectMeta{Name: "workspace1"}},
		},
		{
			name: "fetchWorkspace returns empty list",
			mockClient: MockInterface{
				FakeLoft: &FakeLoft{
					DevPodWorkspaceInstanceList: &managementv1.DevPodWorkspaceInstanceList{},
				},
			},
			expectedError: "could not find corresponding workspace",
		},
		{
			name: "fetchWorkspace returns an error",
			mockClient: MockInterface{
				FakeLoft: &FakeLoft{
					Err: errors.New("an unexpected error"),
				},
			},
			expectedError: "an unexpected error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := &ImportCmd{}
			result, err := cmd.fetchWorkspace(ctx, test.mockClient, workspaceUID, projectName)
			if err != nil && err.Error() != test.expectedError {
				t.Fatalf("expected error %v, got %v", test.expectedError, err)
			}
			reflect.DeepEqual(result, test.expectedResult)
			if !equal(result, test.expectedResult) {
				t.Fatalf("expected result %v, got %v", test.expectedResult, result)
			}
		})
	}
}

type MockInterface struct {
	kubernetes.Interface
	FakeLoft *FakeLoft
}

func (m MockInterface) Agent() agentloftclient.Interface {
	return nil
}

func (m MockInterface) Loft() loftclient.Interface {
	return m.FakeLoft
}

type FakeLoft struct {
	loftclient.Interface
	DevPodWorkspaceInstanceList *managementv1.DevPodWorkspaceInstanceList
	Err                         error
}

func (f *FakeLoft) ManagementV1() v1.ManagementV1Interface {
	return &FakeManagementV1{
		DevPodWorkspaceInstanceList: f.DevPodWorkspaceInstanceList,
		Err:                         f.Err,
	}
}

type FakeManagementV1 struct {
	v1.ManagementV1Interface
	DevPodWorkspaceInstanceList *managementv1.DevPodWorkspaceInstanceList
	Err                         error
}

func (f *FakeManagementV1) DevPodWorkspaceInstances(_ string) v1.DevPodWorkspaceInstanceInterface {
	return &FakeDevPodWorkspaceInstances{
		WorkspaceList: f.DevPodWorkspaceInstanceList,
		Err:           f.Err,
	}
}

type FakeDevPodWorkspaceInstances struct {
	v1.DevPodWorkspaceInstanceInterface
	WorkspaceList *managementv1.DevPodWorkspaceInstanceList
	Err           error
}

func (f *FakeDevPodWorkspaceInstances) List(_ context.Context,
	_ metav1.ListOptions) (*managementv1.DevPodWorkspaceInstanceList, error) {
	return f.WorkspaceList, f.Err
}

// A simple helper function to compare two DevPodWorkspaceInstances
func equal(a, b *managementv1.DevPodWorkspaceInstance) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name
}
