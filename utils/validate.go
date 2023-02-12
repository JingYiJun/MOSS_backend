package utils

import (
	"encoding/json"
	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"reflect"
	"strings"
)

type ErrorDetailElement struct {
	validator.FieldError
	Field string `json:"field"`
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

type ErrorDetail []*ErrorDetailElement

func (e ErrorDetail) Error() string {
	var builder strings.Builder
	builder.WriteString("Validation Error: ")
	for _, err := range e {
		builder.WriteString("invalid " + err.Field)
		builder.WriteString("\n")
	}
	return builder.String()
}

var validate = validator.New()

func init() {
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]

		if name == "-" {
			return ""
		}

		return name
	})
}

func Validate(model any) error {
	errors := validate.Struct(model)
	if errors != nil {
		var errorDetail ErrorDetail
		for _, err := range errors.(validator.ValidationErrors) {
			detail := ErrorDetailElement{
				FieldError: err,
				Field:      err.Field(),
				Tag:        err.Tag(),
				Value:      err.Param(),
			}
			errorDetail = append(errorDetail, &detail)
		}
		return &errorDetail
	}
	return nil
}

func ValidateQuery(c *fiber.Ctx, model any) error {
	err := c.QueryParser(model)
	if err != nil {
		return err
	}
	err = defaults.Set(model)
	if err != nil {
		return err
	}
	return Validate(model)
}

// ValidateBody supports json only
func ValidateBody(c *fiber.Ctx, model any) error {
	body := c.Body()
	if len(body) == 0 {
		body = []byte("{}")
	}
	err := json.Unmarshal(body, model)
	if err != nil {
		return err
	}
	err = defaults.Set(model)
	if err != nil {
		return err
	}
	return Validate(model)
}
