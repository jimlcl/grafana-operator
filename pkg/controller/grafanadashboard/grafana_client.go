package grafanadashboard

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	grafanaapi "github.com/nytm/go-grafana-api"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

const (
	DeleteDashboardByUIDUrl    = "%v/api/dashboards/uid/%v"
	CreateOrUpdateDashboardUrl = "%v/api/dashboards/db"
)

type GrafanaRequest struct {
	Dashboard json.RawMessage `json:"dashboard"`
	FolderId  int64           `json:"folderId"`
	Overwrite bool            `json:"overwrite"`
}

type GrafanaResponse struct {
	ID      *uint   `json:"id"`
	OrgID   *uint   `json:"orgId"`
	Message *string `json:"message"`
	Slug    *string `json:"slug"`
	Version *int    `json:"version"`
	Status  *string `json:"resp"`
	UID     *string `json:"uid"`
	URL     *string `json:"url"`
}

type GrafanaClient interface {
	CreateOrUpdateDashboard(dashboard []byte, folderID int64) (GrafanaResponse, error)
	DeleteDashboardByUID(UID string) (GrafanaResponse, error)
	GetOrCreateNamespaceFolder(namespace string) (int64, error)
}

type GrafanaClientImpl struct {
	url      string
	user     string
	password string
	client   *http.Client
}

func setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "grafana-operator")
}

func NewGrafanaClient(url, user, password string, timeoutSeconds time.Duration) GrafanaClient {
	transport := http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: &transport,
		Timeout:   time.Second * timeoutSeconds,
	}

	return &GrafanaClientImpl{
		url:      url,
		user:     user,
		password: password,
		client:   client,
	}
}

func (r *GrafanaClientImpl) GetOrCreateNamespaceFolder(namespace string) (folderID int64, err error) {
	gapiClient, err := grafanaapi.New("", r.url) // get the Auth from somewhere?
	if err != nil {
		return 0, err
	}
	folders, err := gapiClient.Folders()
	if err != nil {
		return 0, err

	}
	for _, folder := range folders {
		if folder.Title == namespace {
			return folder.Id, nil
		}
		continue
	}
	// Create new folder using the passed namespace and return it's ID
	newFolder, err := gapiClient.NewFolder(namespace)
	if err != nil {
		return 0, err
	}
	return newFolder.Id, nil
}

// Submit dashboard json to grafana
func (r *GrafanaClientImpl) CreateOrUpdateDashboard(dashboard []byte, folderID int64) (GrafanaResponse, error) {
	rawUrl := fmt.Sprintf(CreateOrUpdateDashboardUrl, r.url)
	response := newResponse()

	parsed, err := url.Parse(rawUrl)
	if err != nil {
		return response, err
	}

	// Grafana expects some additional data along with the dashboard
	raw, err := json.Marshal(GrafanaRequest{
		Dashboard: dashboard,

		FolderId: folderID,

		// We always want to set `overwrite` because the uids in the CRs map
		// directly to dashboards in grafana
		Overwrite: true,
	})
	if err != nil {
		return response, err
	}

	parsed.User = url.UserPassword(r.user, r.password)
	req, err := http.NewRequest("POST", parsed.String(), bytes.NewBuffer(raw))
	if err != nil {
		return response, err
	}

	setHeaders(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return response, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return response, errors.New(fmt.Sprintf(
			"error creating dashboard, expected status 200 but got %v",
			resp.StatusCode))
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return response, err
	}

	err = json.Unmarshal(data, &response)
	return response, err
}

// Delete a dashboard given by a UID
func (r *GrafanaClientImpl) DeleteDashboardByUID(UID string) (GrafanaResponse, error) {
	rawUrl := fmt.Sprintf(DeleteDashboardByUIDUrl, r.url, UID)
	response := newResponse()

	parsed, err := url.Parse(rawUrl)
	if err != nil {
		return response, err
	}

	parsed.User = url.UserPassword(r.user, r.password)
	req, err := http.NewRequest("DELETE", parsed.String(), nil)
	if err != nil {
		return response, err
	}

	setHeaders(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return response, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return response, errors.New(fmt.Sprintf(
			"error deleting dashboard, expected status 200 but got %v",
			resp.StatusCode))
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return response, err
	}

	err = json.Unmarshal(data, &response)
	return response, err
}

func newResponse() GrafanaResponse {
	var id uint = 0
	var orgId uint = 0
	var version int = 0
	var status = "(empty)"
	var message = "(empty)"
	var slug string
	var uid string
	var url string

	return GrafanaResponse{
		ID:      &id,
		OrgID:   &orgId,
		Message: &message,
		Slug:    &slug,
		Version: &version,
		Status:  &status,
		UID:     &uid,
		URL:     &url,
	}
}
