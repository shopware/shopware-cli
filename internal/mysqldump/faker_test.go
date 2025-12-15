package mysqldump

import (
	"testing"
)

const (
	Faker = "faker"
)

func Test_service_ReplaceStringWithFakerWhenRequested(t *testing.T) {
	t.Parallel()
	type args struct {
		request string
	}
	tests := []struct {
		name    string
		args    args
		want    func(s interface{}) bool
		errMsg  string
		wantErr bool
	}{
		{
			"Get a name string",
			args{
				"{{- faker.Person().Name() -}}",
			},
			func(s interface{}) bool {
				return len(s.(string)) > 2
			},
			"min len 2",
			false,
		},
		{
			"Get random text with len 10",
			args{
				"{{- faker.Lorem().Text(100) -}}",
			},
			func(s interface{}) bool {
				return len(s.(string)) > 10
			},
			"more that len 10",
			false,
		},
		{
			"asciify something",
			args{
				"{{- faker.Asciify(\"************\") -}}",
			},
			func(s interface{}) bool {
				return len(s.(string)) > 9
			},
			"more that len 9",
			false,
		},
		{
			"skip faker",
			args{
				"dont go to faker",
			},
			func(s interface{}) bool {
				return s.(string) == "dont go to faker"
			},
			"this should have skipped faker",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				t.Parallel()
				got := replaceStringWithFakerWhenRequested(tt.args.request)
				if !tt.want(got) {
					t.Errorf("ReplaceStringWithFakerWhenRequested() got = %v, %s", got, tt.errMsg)
				}
			},
		)
	}
}

func Test_service_FakerError(t *testing.T) {
	t.Parallel()

	type args struct {
		request string
	}
	tests := []struct {
		name    string
		args    args
		want    func(s interface{}) bool
		errMsg  string
		wantErr bool
	}{
		{
			"pass faker without context",
			args{
				Faker,
			},
			func(s interface{}) bool {
				return true
			},
			"this should have errored",
			true,
		},
		{
			"try calling missing method",
			args{
				"faker.Yada()",
			},
			func(s interface{}) bool {
				return true
			},
			"this should have errored",
			true,
		},
		{
			"try calling missing method on last level",
			args{
				"faker.Person().Yada()",
			},
			func(s interface{}) bool {
				return true
			},
			"this should have errored",
			true,
		},
		{
			"calling function with diff number of args that supposed",
			args{
				"faker.Person().Name(\"batatinhas\")",
			},
			func(s interface{}) bool {
				return true
			},
			"this should have errored",
			true,
		},
		{
			"calling ContactInfo which is not supported",
			args{
				"faker.ContactInfo()",
			},
			func(s interface{}) bool {
				return true
			},
			"this should have errored",
			true,
		},
		{
			"calling function with diff number of args that supposed",
			args{
				"faker.Person().Name(\"batatinhas\")",
			},
			func(s interface{}) bool {
				return true
			},
			"this should have errored",
			true,
		},
		{
			"call a property",
			args{
				"faker.Bla",
			},
			func(s interface{}) bool {
				return true
			},
			"this should have errored",
			true,
		},
		{
			"started a function call but missing parameters",
			args{
				"faker.Bla(",
			},
			func(s interface{}) bool {
				return true
			},
			"this should have errored",
			true,
		},

		{
			"missing parameters on long call",
			args{
				"faker.Bla().Fuu",
			},
			func(s interface{}) bool {
				return true
			},
			"this should have errored",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				t.Parallel()
				got, err := evaluateFakerExpression(tt.args.request)
				if (err != nil) != tt.wantErr {
					t.Errorf("ReplaceStringWithFakerWhenRequested() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !tt.want(got) {
					t.Errorf("ReplaceStringWithFakerWhenRequested() got = %v, %s", got, tt.errMsg)
				}
			},
		)
	}
}
