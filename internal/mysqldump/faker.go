package mysqldump

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/jaswdr/faker/v2"
)

var fakerInstance = faker.New()

const (
	fakerShortLen = 2
	contactInfo   = "ContactInfo"
)

func replaceStringWithFakerWhenRequested(request string) (string, error) {
	if len(request) < 5 || request[0:5] != "faker" {
		return request, nil
	}

	a := strings.Split(request, ".")
	if len(a) < 2 {
		return request,
			errors.New("requires more arguments to get a faker func")
	}

	var res []reflect.Value
	var err error

	method := strings.Split(a[1], "(")[0]
	if method == contactInfo {
		return request, errors.New("method ContactInfo is not supported")
	}

	if len(a) == fakerShortLen {
		argsString := strings.Split(a[1], "(")

		if len(argsString) < 2 {
			return request,
				errors.New("requires more arguments to get a faker func")
		}

		args, dErr := getMethodArguments(
			&fakerInstance,
			argsString[0],
			strings.Replace(argsString[1], ")", "", -1),
		)

		if dErr != nil {
			return "", dErr
		}

		if res, dErr = callMethod(&fakerInstance, method, args); dErr != nil {
			return "", dErr
		}

		return stringify(res[0]), nil
	}

	if res, err = callMethod(&fakerInstance, method, nil); err != nil {
		return "", err
	}

	argsString := strings.Split(a[2], "(")

	if len(argsString) < 2 {
		return request,
			errors.New("requires more arguments to get a faker func")
	}

	obj := reflect.Indirect(res[0]).Interface()

	args, err := getMethodArguments(
		obj,
		argsString[0],
		strings.Replace(argsString[1], ")", "", -1),
	)
	if err != nil {
		return "", err
	}
	// faker as a max depth of 2, here should be last
	// @todo this analysis should be dynamic
	if res, err = callMethod(obj, argsString[0], args); err != nil {
		return "", err
	}

	return stringify(res[0]), nil
}

func getMethodArguments(
	entry interface{},
	name string,
	argsString string,
) ([]reflect.Value, error) {
	_, exists := reflect.TypeOf(entry).MethodByName(name)

	if !exists {
		return nil,
			errors.New("could not find faker func")
	}

	method := reflect.ValueOf(entry).
		MethodByName(name)

	in := make([]reflect.Value, method.Type().NumIn())

	// we need to skip if we have an empty string, else it fails the lower validation
	// and function goes kaboom
	if argsString == "" {
		return in, nil
	}

	args := strings.Split(argsString, ",")

	if len(args) != method.Type().NumIn() {
		return nil, fmt.Errorf(
			"number of arguments to call %s is %d and we got %d",
			name,
			method.Type().NumIn(),
			len(args),
		)
	}

	for i := 0; i < method.Type().NumIn(); i++ {
		t := method.Type().In(i)
		in[i] = reflect.ValueOf(convert(t, clearString(args[i]))).Convert(t)
	}

	return in, nil
}

func callMethod(
	entry interface{},
	name string,
	objects []reflect.Value,
) ([]reflect.Value, error) {
	_, exists := reflect.TypeOf(entry).MethodByName(name)

	if !exists {
		return nil,
			errors.New("could not find faker func")
	}

	return reflect.ValueOf(entry).
		MethodByName(name).
		Call(objects), nil
}

func clearString(str string) string {
	s := strings.Replace(str, "\"", "", -1)
	return strings.Replace(s, " ", "", -1)
}

func stringify(v interface{}) string {
	return fmt.Sprintf("%v", v)
}

func convert(t reflect.Type, s string) interface{} {
	// nolint
	switch t.Kind() {
	case reflect.Int, reflect.Int64:
		v, _ := strconv.Atoi(s)
		return v
	default:
		return s
	}
}
