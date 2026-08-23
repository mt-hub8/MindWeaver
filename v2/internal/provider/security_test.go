package provider

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSerializableContractsHaveNoSensitiveFieldsOrStringMethods(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(Descriptor{}),
		reflect.TypeOf(ModelDescriptor{}),
		reflect.TypeOf(CapabilitySet{}),
		reflect.TypeOf(ModelConstraints{}),
		reflect.TypeOf(RequestProfile{}),
		reflect.TypeOf(Endpoint{}),
		reflect.TypeOf(EgressRule{}),
		reflect.TypeOf(EgressRequest{}),
		reflect.TypeOf(EgressDecision{}),
		reflect.TypeOf(InferenceInvocation{}),
		reflect.TypeOf(InvocationFailure{}),
		reflect.TypeOf(InvocationRefusal{}),
		reflect.TypeOf(InvocationCancellation{}),
		reflect.TypeOf(InvocationUsage{}),
	}
	forbidden := []string{
		"apikey", "secret", "password", "credential", "authorization",
		"httpheader", "prompt", "requestbody", "responsebody", "rawpayload",
		"rawerror", "generatedcontent",
	}
	packagePath := reflect.TypeOf(Descriptor{}).PkgPath()
	seen := make(map[reflect.Type]bool)
	for _, contract := range types {
		assertNoSensitiveFields(t, contract, packagePath, forbidden, seen)
		pointer := reflect.New(contract).Interface()
		if _, implementsStringer := pointer.(fmt.Stringer); implementsStringer {
			t.Errorf("%s must not implement fmt.Stringer", contract.Name())
		}
	}
}

func TestEndpointErrorsNeverEchoRejectedCredential(t *testing.T) {
	const credential = "super-secret-credential"
	_, err := ParseEndpoint("https://user:" + credential + "@example.com/v1")
	if err == nil {
		t.Fatal("credential-bearing URL accepted")
	}
	if strings.Contains(err.Error(), credential) {
		t.Fatalf("endpoint error leaked credential: %v", err)
	}
}

func assertNoSensitiveFields(t *testing.T, current reflect.Type, packagePath string, forbidden []string, seen map[reflect.Type]bool) {
	t.Helper()
	for current.Kind() == reflect.Pointer || current.Kind() == reflect.Slice || current.Kind() == reflect.Array {
		current = current.Elem()
	}
	if current.Kind() != reflect.Struct || current.PkgPath() != packagePath || seen[current] {
		return
	}
	seen[current] = true
	for index := 0; index < current.NumField(); index++ {
		field := current.Field(index)
		if !field.IsExported() {
			continue
		}
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		searchable := normalizeFieldName(field.Name + jsonName)
		for _, fragment := range forbidden {
			if strings.Contains(searchable, fragment) {
				t.Errorf("serializable field %s.%s contains forbidden sensitive concept %q", current.Name(), field.Name, fragment)
			}
		}
		assertNoSensitiveFields(t, field.Type, packagePath, forbidden, seen)
	}
}

func normalizeFieldName(value string) string {
	value = strings.ToLower(value)
	replacer := strings.NewReplacer("_", "", "-", "")
	return replacer.Replace(value)
}
