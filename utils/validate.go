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
	Field string                `json:"field"`
	Tag   string                `json:"tag"`
	Value string                `json:"value"`
	Error *validator.FieldError `json:"-"`
}

type ErrorDetail []*ErrorDetailElement

func (e *ErrorDetail) Error() string {
	return "Validation Error"
	//var builder strings.Builder
	//for _, err := range *e {
	//	builder.WriteString((*err.Error).Error())
	//	builder.WriteString("\n")
	//}
	//return builder.String()
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
				Field: err.Field(),
				Tag:   err.Tag(),
				Value: err.Param(),
				Error: &err,
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
