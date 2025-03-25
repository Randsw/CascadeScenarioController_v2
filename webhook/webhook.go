package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/randsw/cascadescenariocontroller/logger"
)

func SendWebHook(message string, address string) (string, error) {
	values := map[string]string{"message": message}
	jsonData, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	resp, err := http.Post(address, "application/json",
		bytes.NewBuffer(jsonData))

	if err != nil {
		return "", err
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			logger.Error("Cant close response body")
		}
	}()
	return strconv.Itoa(resp.StatusCode), nil
}
