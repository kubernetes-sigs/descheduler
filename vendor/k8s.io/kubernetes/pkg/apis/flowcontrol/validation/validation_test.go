/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validation

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/flowcontrol"
)

func TestFlowSchemaValidation(t *testing.T) {
	testCases := []struct {
		name           string
		flowSchema     *flowcontrol.FlowSchema
		expectedErrors field.ErrorList
	}{
		{
			name: "missing both resource and non-resource policy-rule should fail",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Required(field.NewPath("spec").Child("rules").Index(0), "at least one of resourceRules and nonResourceRules has to be non-empty"),
			},
		},
		{
			name: "normal flow-schema w/ * verbs/apiGroups/resources should work",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind:  flowcontrol.SubjectKindGroup,
									Group: &flowcontrol.GroupSubject{Name: "noxu"},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:      []string{flowcontrol.VerbAll},
									APIGroups:  []string{flowcontrol.APIGroupAll},
									Resources:  []string{flowcontrol.ResourceAll},
									Namespaces: []string{flowcontrol.NamespaceEvery},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "flow-schema mixes * verbs/apiGroups/resources should fail",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:      []string{flowcontrol.VerbAll, "create"},
									APIGroups:  []string{flowcontrol.APIGroupAll, "tak"},
									Resources:  []string{flowcontrol.ResourceAll, "tok"},
									Namespaces: []string{flowcontrol.NamespaceEvery},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("rules").Index(0).Child("resourceRules").Index(0).Child("verbs"), []string{"*", "create"}, "if '*' is present, must not specify other verbs"),
				field.Invalid(field.NewPath("spec").Child("rules").Index(0).Child("resourceRules").Index(0).Child("apiGroups"), []string{"*", "tak"}, "if '*' is present, must not specify other api groups"),
				field.Invalid(field.NewPath("spec").Child("rules").Index(0).Child("resourceRules").Index(0).Child("resources"), []string{"*", "tok"}, "if '*' is present, must not specify other resources"),
			},
		},
		{
			name: "flow-schema has both resource rules and non-resource rules should work",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:      []string{flowcontrol.VerbAll},
									APIGroups:  []string{flowcontrol.APIGroupAll},
									Resources:  []string{flowcontrol.ResourceAll},
									Namespaces: []string{flowcontrol.NamespaceEvery},
								},
							},
							NonResourceRules: []flowcontrol.NonResourcePolicyRule{
								{
									Verbs:           []string{flowcontrol.VerbAll},
									NonResourceURLs: []string{"/apis/*"},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "flow-schema mixes * non-resource URLs should fail",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
							NonResourceRules: []flowcontrol.NonResourcePolicyRule{
								{
									Verbs:           []string{"*"},
									NonResourceURLs: []string{flowcontrol.NonResourceAll, "tik"},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("rules").Index(0).Child("nonResourceRules").Index(0).Child("nonResourceURLs"), []string{"*", "tik"}, "if '*' is present, must not specify other non-resource URLs"),
			},
		},
		{
			name: "invalid subject kind should fail",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: "FooKind",
								},
							},
							NonResourceRules: []flowcontrol.NonResourcePolicyRule{
								{
									Verbs:           []string{"*"},
									NonResourceURLs: []string{flowcontrol.NonResourceAll},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.NotSupported(field.NewPath("spec").Child("rules").Index(0).Child("subjects").Index(0).Child("kind"), flowcontrol.SubjectKind("FooKind"), supportedSubjectKinds.List()),
			},
		},
		{
			name: "flow-schema w/ invalid verb should fail",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:      []string{"feed"},
									APIGroups:  []string{flowcontrol.APIGroupAll},
									Resources:  []string{flowcontrol.ResourceAll},
									Namespaces: []string{flowcontrol.NamespaceEvery},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.NotSupported(field.NewPath("spec").Child("rules").Index(0).Child("resourceRules").Index(0).Child("verbs"), []string{"feed"}, supportedVerbs.List()),
			},
		},
		{
			name: "flow-schema w/ invalid priority level configuration name should fail",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system+++$$",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:      []string{flowcontrol.VerbAll},
									APIGroups:  []string{flowcontrol.APIGroupAll},
									Resources:  []string{flowcontrol.ResourceAll},
									Namespaces: []string{flowcontrol.NamespaceEvery},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("priorityLevelConfiguration").Child("name"), "system+++$$", `a DNS-1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')`),
			},
		},
		{
			name: "flow-schema w/ service-account kind missing namespace should fail",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindServiceAccount,
									ServiceAccount: &flowcontrol.ServiceAccountSubject{
										Name: "noxu",
									},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:      []string{flowcontrol.VerbAll},
									APIGroups:  []string{flowcontrol.APIGroupAll},
									Resources:  []string{flowcontrol.ResourceAll},
									Namespaces: []string{flowcontrol.NamespaceEvery},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Required(field.NewPath("spec").Child("rules").Index(0).Child("subjects").Index(0).Child("serviceAccount").Child("namespace"), "must specify namespace for service account"),
			},
		},
		{
			name: "flow-schema missing kind should fail",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: "",
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:      []string{flowcontrol.VerbAll},
									APIGroups:  []string{flowcontrol.APIGroupAll},
									Resources:  []string{flowcontrol.ResourceAll},
									Namespaces: []string{flowcontrol.NamespaceEvery},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.NotSupported(field.NewPath("spec").Child("rules").Index(0).Child("subjects").Index(0).Child("kind"), flowcontrol.SubjectKind(""), supportedSubjectKinds.List()),
			},
		},
		{
			name: "Omitted ResourceRule.Namespaces should fail",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:      []string{flowcontrol.VerbAll},
									APIGroups:  []string{flowcontrol.APIGroupAll},
									Resources:  []string{flowcontrol.ResourceAll},
									Namespaces: nil,
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Required(field.NewPath("spec").Child("rules").Index(0).Child("resourceRules").Index(0).Child("namespaces"), "resource rules that are not cluster scoped must supply at least one namespace"),
			},
		},
		{
			name: "ClusterScope is allowed, with no Namespaces",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:        []string{flowcontrol.VerbAll},
									APIGroups:    []string{flowcontrol.APIGroupAll},
									Resources:    []string{flowcontrol.ResourceAll},
									ClusterScope: true,
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "ClusterScope is allowed with NamespaceEvery",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:        []string{flowcontrol.VerbAll},
									APIGroups:    []string{flowcontrol.APIGroupAll},
									Resources:    []string{flowcontrol.ResourceAll},
									ClusterScope: true,
									Namespaces:   []string{flowcontrol.NamespaceEvery},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "NamespaceEvery may not be combined with particulars",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:      []string{flowcontrol.VerbAll},
									APIGroups:  []string{flowcontrol.APIGroupAll},
									Resources:  []string{flowcontrol.ResourceAll},
									Namespaces: []string{"foo", flowcontrol.NamespaceEvery},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("rules").Index(0).Child("resourceRules").Index(0).Child("namespaces"), []string{"foo", flowcontrol.NamespaceEvery}, "if '*' is present, must not specify other namespaces"),
			},
		},
		{
			name: "ResourceRule.Namespaces must be well formed",
			flowSchema: &flowcontrol.FlowSchema{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.FlowSchemaSpec{
					MatchingPrecedence: 50,
					PriorityLevelConfiguration: flowcontrol.PriorityLevelConfigurationReference{
						Name: "system-bar",
					},
					Rules: []flowcontrol.PolicyRulesWithSubjects{
						{
							Subjects: []flowcontrol.Subject{
								{
									Kind: flowcontrol.SubjectKindUser,
									User: &flowcontrol.UserSubject{Name: "noxu"},
								},
							},
							ResourceRules: []flowcontrol.ResourcePolicyRule{
								{
									Verbs:      []string{flowcontrol.VerbAll},
									APIGroups:  []string{flowcontrol.APIGroupAll},
									Resources:  []string{flowcontrol.ResourceAll},
									Namespaces: []string{"-foo"},
								},
							},
						},
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("rules").Index(0).Child("resourceRules").Index(0).Child("namespaces").Index(0), "-foo", nsErrIntro+`a DNS-1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character (e.g. 'my-name',  or '123-abc', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')`),
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			errs := ValidateFlowSchema(testCase.flowSchema)
			if !assert.ElementsMatch(t, testCase.expectedErrors, errs) {
				t.Logf("mismatch: %v", cmp.Diff(testCase.expectedErrors, errs))
			}
		})
	}
}

func TestPriorityLevelConfigurationValidation(t *testing.T) {
	testCases := []struct {
		name                       string
		priorityLevelConfiguration *flowcontrol.PriorityLevelConfiguration
		expectedErrors             field.ErrorList
	}{
		{
			name: "normal customized priority level should work",
			priorityLevelConfiguration: &flowcontrol.PriorityLevelConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.PriorityLevelConfigurationSpec{
					Type: flowcontrol.PriorityLevelEnablementLimited,
					Limited: &flowcontrol.LimitedPriorityLevelConfiguration{
						AssuredConcurrencyShares: 100,
						LimitResponse: flowcontrol.LimitResponse{
							Type: flowcontrol.LimitResponseTypeQueue,
							Queuing: &flowcontrol.QueuingConfiguration{
								Queues:           512,
								HandSize:         4,
								QueueLengthLimit: 100,
							}}}},
			},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "system low priority level w/ exempt should work",
			priorityLevelConfiguration: &flowcontrol.PriorityLevelConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: flowcontrol.PriorityLevelConfigurationNameExempt,
				},
				Spec: flowcontrol.PriorityLevelConfigurationSpec{
					Type: flowcontrol.PriorityLevelEnablementExempt,
				},
			},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "customized priority level w/ overflowing handSize/queues should fail 1",
			priorityLevelConfiguration: &flowcontrol.PriorityLevelConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.PriorityLevelConfigurationSpec{
					Type: flowcontrol.PriorityLevelEnablementLimited,
					Limited: &flowcontrol.LimitedPriorityLevelConfiguration{
						AssuredConcurrencyShares: 100,
						LimitResponse: flowcontrol.LimitResponse{
							Type: flowcontrol.LimitResponseTypeQueue,
							Queuing: &flowcontrol.QueuingConfiguration{
								QueueLengthLimit: 100,
								Queues:           512,
								HandSize:         8,
							}}}},
			},
			expectedErrors: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("limited").Child("limitResponse").Child("queuing").Child("handSize"), int32(8), "required entropy bits of deckSize 512 and handSize 8 should not be greater than 60"),
			},
		},
		{
			name: "customized priority level w/ overflowing handSize/queues should fail 2",
			priorityLevelConfiguration: &flowcontrol.PriorityLevelConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.PriorityLevelConfigurationSpec{
					Type: flowcontrol.PriorityLevelEnablementLimited,
					Limited: &flowcontrol.LimitedPriorityLevelConfiguration{
						AssuredConcurrencyShares: 100,
						LimitResponse: flowcontrol.LimitResponse{
							Type: flowcontrol.LimitResponseTypeQueue,
							Queuing: &flowcontrol.QueuingConfiguration{
								QueueLengthLimit: 100,
								Queues:           128,
								HandSize:         10,
							}}}},
			},
			expectedErrors: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("limited").Child("limitResponse").Child("queuing").Child("handSize"), int32(10), "required entropy bits of deckSize 128 and handSize 10 should not be greater than 60"),
			},
		},
		{
			name: "customized priority level w/ overflowing handSize/queues should fail 3",
			priorityLevelConfiguration: &flowcontrol.PriorityLevelConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.PriorityLevelConfigurationSpec{
					Type: flowcontrol.PriorityLevelEnablementLimited,
					Limited: &flowcontrol.LimitedPriorityLevelConfiguration{
						AssuredConcurrencyShares: 100,
						LimitResponse: flowcontrol.LimitResponse{
							Type: flowcontrol.LimitResponseTypeQueue,
							Queuing: &flowcontrol.QueuingConfiguration{
								QueueLengthLimit: 100,
								Queues:           math.MaxInt32,
								HandSize:         3,
							}}}},
			},
			expectedErrors: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("limited").Child("limitResponse").Child("queuing").Child("handSize"), int32(3), "required entropy bits of deckSize 2147483647 and handSize 3 should not be greater than 60"),
				field.Invalid(field.NewPath("spec").Child("limited").Child("limitResponse").Child("queuing").Child("queues"), int32(math.MaxInt32), "must not be greater than 10000000"),
			},
		},
		{
			name: "customized priority level w/ handSize=2 and queues=10^7 should work",
			priorityLevelConfiguration: &flowcontrol.PriorityLevelConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.PriorityLevelConfigurationSpec{
					Type: flowcontrol.PriorityLevelEnablementLimited,
					Limited: &flowcontrol.LimitedPriorityLevelConfiguration{
						AssuredConcurrencyShares: 100,
						LimitResponse: flowcontrol.LimitResponse{
							Type: flowcontrol.LimitResponseTypeQueue,
							Queuing: &flowcontrol.QueuingConfiguration{
								QueueLengthLimit: 100,
								Queues:           10 * 1000 * 1000, // 10^7
								HandSize:         2,
							}}}},
			},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "customized priority level w/ handSize greater than queues should fail",
			priorityLevelConfiguration: &flowcontrol.PriorityLevelConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "system-foo",
				},
				Spec: flowcontrol.PriorityLevelConfigurationSpec{
					Type: flowcontrol.PriorityLevelEnablementLimited,
					Limited: &flowcontrol.LimitedPriorityLevelConfiguration{
						AssuredConcurrencyShares: 100,
						LimitResponse: flowcontrol.LimitResponse{
							Type: flowcontrol.LimitResponseTypeQueue,
							Queuing: &flowcontrol.QueuingConfiguration{
								QueueLengthLimit: 100,
								Queues:           7,
								HandSize:         8,
							}}}},
			},
			expectedErrors: field.ErrorList{
				field.Invalid(field.NewPath("spec").Child("limited").Child("limitResponse").Child("queuing").Child("handSize"), int32(8), "should not be greater than queues (7)"),
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			errs := ValidatePriorityLevelConfiguration(testCase.priorityLevelConfiguration)
			if !assert.ElementsMatch(t, testCase.expectedErrors, errs) {
				t.Logf("mismatch: %v", cmp.Diff(testCase.expectedErrors, errs))
			}
		})
	}
}

func TestValidateFlowSchemaStatus(t *testing.T) {
	testCases := []struct {
		name           string
		status         *flowcontrol.FlowSchemaStatus
		expectedErrors field.ErrorList
	}{
		{
			name:           "empty status should work",
			status:         &flowcontrol.FlowSchemaStatus{},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "duplicate key should fail",
			status: &flowcontrol.FlowSchemaStatus{
				Conditions: []flowcontrol.FlowSchemaCondition{
					{
						Type: "1",
					},
					{
						Type: "1",
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Duplicate(field.NewPath("status").Child("conditions").Index(1).Child("type"), flowcontrol.FlowSchemaConditionType("1")),
			},
		},
		{
			name: "missing key should fail",
			status: &flowcontrol.FlowSchemaStatus{
				Conditions: []flowcontrol.FlowSchemaCondition{
					{
						Type: "",
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Required(field.NewPath("status").Child("conditions").Index(0).Child("type"), "must not be empty"),
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			errs := ValidateFlowSchemaStatus(testCase.status, field.NewPath("status"))
			if !assert.ElementsMatch(t, testCase.expectedErrors, errs) {
				t.Logf("mismatch: %v", cmp.Diff(testCase.expectedErrors, errs))
			}
		})
	}
}

func TestValidatePriorityLevelConfigurationStatus(t *testing.T) {
	testCases := []struct {
		name           string
		status         *flowcontrol.PriorityLevelConfigurationStatus
		expectedErrors field.ErrorList
	}{
		{
			name:           "empty status should work",
			status:         &flowcontrol.PriorityLevelConfigurationStatus{},
			expectedErrors: field.ErrorList{},
		},
		{
			name: "duplicate key should fail",
			status: &flowcontrol.PriorityLevelConfigurationStatus{
				Conditions: []flowcontrol.PriorityLevelConfigurationCondition{
					{
						Type: "1",
					},
					{
						Type: "1",
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Duplicate(field.NewPath("status").Child("conditions").Index(1).Child("type"), flowcontrol.PriorityLevelConfigurationConditionType("1")),
			},
		},
		{
			name: "missing key should fail",
			status: &flowcontrol.PriorityLevelConfigurationStatus{
				Conditions: []flowcontrol.PriorityLevelConfigurationCondition{
					{
						Type: "",
					},
				},
			},
			expectedErrors: field.ErrorList{
				field.Required(field.NewPath("status").Child("conditions").Index(0).Child("type"), "must not be empty"),
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			errs := ValidatePriorityLevelConfigurationStatus(testCase.status, field.NewPath("status"))
			if !assert.ElementsMatch(t, testCase.expectedErrors, errs) {
				t.Logf("mismatch: %v", cmp.Diff(testCase.expectedErrors, errs))
			}
		})
	}
}

func TestValidateNonResourceURLPath(t *testing.T) {
	testCases := []struct {
		name           string
		path           string
		expectingError bool
	}{
		{
			name:           "empty string should fail",
			path:           "",
			expectingError: true,
		},
		{
			name:           "no slash should fail",
			path:           "foo",
			expectingError: true,
		},
		{
			name:           "single slash should work",
			path:           "/",
			expectingError: false,
		},
		{
			name:           "continuous slash should fail",
			path:           "//",
			expectingError: true,
		},
		{
			name:           "/foo slash should work",
			path:           "/foo",
			expectingError: false,
		},
		{
			name:           "multiple continuous slashes should fail",
			path:           "/////",
			expectingError: true,
		},
		{
			name:           "ending up with slash should work",
			path:           "/apis/",
			expectingError: false,
		},
		{
			name:           "ending up with wildcard should work",
			path:           "/healthz/*",
			expectingError: false,
		},
		{
			name:           "single wildcard inside the path should fail",
			path:           "/healthz/*/foo",
			expectingError: true,
		},
		{
			name:           "white-space in the path should fail",
			path:           "/healthz/foo bar",
			expectingError: true,
		},
		{
			name:           "wildcard plus plain path should fail",
			path:           "/health*",
			expectingError: true,
		},
		{
			name:           "wildcard plus plain path should fail 2",
			path:           "/health*/foo",
			expectingError: true,
		},
		{
			name:           "multiple wildcard internal and suffix should fail",
			path:           "/*/*",
			expectingError: true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateNonResourceURLPath(testCase.path, field.NewPath(""))
			assert.Equal(t, testCase.expectingError, err != nil,
				"actual error: %v", err)
		})
	}
}
