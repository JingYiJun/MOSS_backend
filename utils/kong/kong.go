package kong

import (
	"MOSS_backend/config"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"strconv"
)

type JwtCredential struct {
	ID        string `json:"id"`
	Secret    string `json:"secret"`
	Key       string `json:"key"`
	Algorithm string `json:"algorithm"`
}

type JwtCredentials struct {
	Next string           `json:"next"`
	Data []*JwtCredential `json:"data"`
}

var kongClient = &http.Client{}

func kongRequestDo(Method, URI string, body io.Reader, contentType string) (int, []byte, error) {
	req, err := http.NewRequest(
		Method,
		fmt.Sprintf("%v%v", config.Config.KongUrl, URI),
		body,
	)
	if err != nil {
		return 500, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rsp, err := kongClient.Do(req)
	defer func() {
		_ = rsp.Body.Close()
	}()
	if err != nil {
		return 500, nil, err
	}
	data, err := io.ReadAll(rsp.Body)
	return rsp.StatusCode, data, err
}

func Ping() error {
	req, err := kongClient.Get(config.Config.KongUrl)
	if err != nil {
		return err
	}

	if req.StatusCode != 200 {
		return fmt.Errorf("error connect to kong[%s]: %s", config.Config.KongUrl, err)
	} else {
		fmt.Println("ping kong success")
	}
	return req.Body.Close()
}

func CreateUser(userID int) error {
	reqBodyObject := map[string]any{
		"username": strconv.Itoa(userID),
	}
	reqData, err := json.Marshal(reqBodyObject)
	if err != nil {
		return err
	}
	statusCode, body, err := kongRequestDo(
		http.MethodPut,
		fmt.Sprintf("/consumers/%d", userID),
		bytes.NewReader(reqData),
		fiber.MIMEApplicationJSON,
	)
	if err != nil {
		return err
	}
	if !(statusCode == 200 || statusCode == 201) {
		return fmt.Errorf("create user %v in kong error: %v", userID, string(body))
	}
	return nil
}

func CreateJwtCredential(userID int) (*JwtCredential, error) {
	statusCode, body, err := kongRequestDo(
		http.MethodPost,
		fmt.Sprintf("/consumers/%d/jwt", userID),
		nil,
		fiber.MIMEApplicationForm,
	)
	if err != nil {
		return nil, err
	}
	if statusCode == 404 {
		err = CreateUser(userID)
		return CreateJwtCredential(userID)
	} else if statusCode != 201 {
		return nil, fmt.Errorf("create user %v jwt credential error: %v", userID, string(body))
	}
	jwtCredential := new(JwtCredential)
	err = json.Unmarshal(body, &jwtCredential)
	if err != nil {
		return nil, err
	}
	return jwtCredential, nil
}

func ListJwtCredentials(userID int) ([]*JwtCredential, error) {
	statusCode, body, err := kongRequestDo(
		http.MethodGet,
		fmt.Sprintf("/consumers/%d/jwt", userID),
		nil,
		"",
	)
	if err != nil {
		return nil, err
	}
	if statusCode != 200 {
		return nil, fmt.Errorf("list credential error: %v", string(body))
	}

	var jwtCredentials JwtCredentials
	err = json.Unmarshal(body, &jwtCredentials)
	if err != nil {
		return nil, err
	}

	return jwtCredentials.Data, nil
}

func GetJwtCredential(userID int) (*JwtCredential, error) {
	jwtCredentials, err := ListJwtCredentials(userID)
	if err != nil {
		return nil, err
	}
	if len(jwtCredentials) == 0 {
		return CreateJwtCredential(userID)
	} else {
		return jwtCredentials[0], nil
	}
}

func DeleteJwtCredential(userID int) error {
	deleteAJwtCredential := func(jwtID string) error {
		statusCode, _, err := kongRequestDo(
			http.MethodDelete,
			fmt.Sprintf("/consumers/%d/jwt/%v", userID, jwtID),
			nil,
			"",
		)
		if err != nil {
			return err
		}
		if statusCode != 204 {
			return fmt.Errorf("delete user %v jwt credential %v error", userID, jwtID)
		}
		return nil
	}

	var err error
	jwtCredentials, err := ListJwtCredentials(userID)
	if err != nil {
		return err
	}
	for i := range jwtCredentials {
		innerErr := deleteAJwtCredential(jwtCredentials[i].ID)
		if innerErr != nil {
			err = errors.Wrap(innerErr, err.Error())
		}
	}
	return err
}
