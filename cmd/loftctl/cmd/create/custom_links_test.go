package create

import (
	"testing"

	managementv1 "github.com/loft-sh/api/v3/pkg/apis/management/v1"
	"gotest.tools/v3/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetCustomLinksAnnotation(t *testing.T) {
	type testCase struct {
		desc                string
		obj                 metav1.Object
		links               []string
		expectedAnnotations map[string]string
		shouldChange        bool
	}

	testTable := []testCase{
		{
			desc:                "virtualclusterinstance, links specified",
			obj:                 &managementv1.VirtualClusterInstance{},
			links:               []string{"a=b", "c=d"},
			expectedAnnotations: map[string]string{LoftCustomLinksAnnotation: "a=b\nc=d"},
			shouldChange:        true,
		},
		{
			desc:                "spaceinstance, links specified",
			obj:                 &managementv1.SpaceInstance{},
			links:               []string{"a=b", "c=d"},
			expectedAnnotations: map[string]string{LoftCustomLinksAnnotation: "a=b\nc=d"},
			shouldChange:        true,
		},
		{
			desc:                "virtualclusterinstance, links specified, existing annotations",
			obj:                 &managementv1.VirtualClusterInstance{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"abc": "xyz"}}},
			links:               []string{"a=b", "c=d"},
			expectedAnnotations: map[string]string{"abc": "xyz", LoftCustomLinksAnnotation: "a=b\nc=d"},
			shouldChange:        true,
		},
		{
			desc:                "virtualclusterinstance, links specified, existing annotations with extra spaces, expecting padding to be trimmed",
			obj:                 &managementv1.VirtualClusterInstance{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"abc": "xyz"}}},
			links:               []string{"  a=b   ", "	c=d  "},
			expectedAnnotations: map[string]string{"abc": "xyz", LoftCustomLinksAnnotation: "a=b\nc=d"},
			shouldChange:        true,
		},
		{
			desc:                "spaceinstance, object nil, should not panic",
			obj:                 nil,
			links:               []string{"a=b", "c=d"},
			expectedAnnotations: nil,
			shouldChange:        false,
		},
		{
			desc:                "virtualclusterinstance, case update, should replace",
			obj:                 &managementv1.VirtualClusterInstance{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{LoftCustomLinksAnnotation: "a=b\nc=d"}}},
			links:               []string{"x=y", "l=z"},
			expectedAnnotations: map[string]string{LoftCustomLinksAnnotation: "x=y\nl=z"},
			shouldChange:        true,
		},
		{
			desc:                "virtualclusterinstance, case update, should not replace if empty links",
			obj:                 &managementv1.VirtualClusterInstance{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{LoftCustomLinksAnnotation: "x=y\nl=z"}}},
			links:               []string{},
			expectedAnnotations: map[string]string{LoftCustomLinksAnnotation: "x=y\nl=z"},
			shouldChange:        false,
		},
		{
			desc:                "virtualclusterinstance, case update, links specified new, should not affect existing",
			obj:                 &managementv1.VirtualClusterInstance{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"abc": "xyz"}}},
			links:               []string{"x=y", "l=z"},
			expectedAnnotations: map[string]string{"abc": "xyz", LoftCustomLinksAnnotation: "x=y\nl=z"},
			shouldChange:        true,
		},
		{
			desc:                "virtualclusterinstance, case update, links specified different, should replace, should not affect existing",
			obj:                 &managementv1.VirtualClusterInstance{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"abc": "xyz", LoftCustomLinksAnnotation: "a=b\nc=d"}}},
			links:               []string{"x=y", "l=z"},
			expectedAnnotations: map[string]string{"abc": "xyz", LoftCustomLinksAnnotation: "x=y\nl=z"},
			shouldChange:        true,
		},
	}

	for i, tc := range testTable {
		t.Logf("Test Case #%d: %q\n", i, tc.desc)
		changed := SetCustomLinksAnnotation(tc.obj, tc.links)
		assert.Equal(t, changed, tc.shouldChange)
		if tc.obj == nil {
			assert.Equal(t, tc.obj, nil)
			// don't try to compare if the test object is nil
			return
		}
		annotations := tc.obj.GetAnnotations()
		assert.DeepEqual(t, annotations, tc.expectedAnnotations)
	}
}
