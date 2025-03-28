package ecloud

import (
	"net/http"
	"encoding/json"
	"bytes"
	"io"
)


// ------------------------------ API CALLS FUNCTIONS -------------------------

// Retrieve a list of VMs
func (c *Client) GetVm(url string, resType interface{}) error {
	return c.CallAPI("GET", url, nil, resType, true)
}

// Retrieve a VM by its ID
func (c *Client) GetVmById(url string, id string, resType interface{}) error {
	return c.CallAPI("GET", url, nil, resType, true)
}

// ...


// ------------------------------ UTILS FUNCTIONS -----------------------------

// Base function to perform API calls
// Args:
// - method: HTTP method to use
// - path: API path to call
// - reqBody: request body to send
// - resType: response type to unmarshal
// - needAuth: if the call needs authentication
// Returns:
// - error: if any
func (c *Client) CallAPI(method, path string, reqBody, resType interface{}, needAuth bool) error {
	req, err := c.NewRequest(method, path, reqBody, needAuth)
	if err != nil {
		return err
	}
	response, err := c.Do(req)
	if err != nil {
		return err
	}
	return c.UnmarshalResponse(response, resType)
}

// NewRequest returns a new HTTP request
func (c *Client) NewRequest(method, path string, reqBody interface{}, needAuth bool) (*http.Request, error) {
	var body []byte
	var err error

	if reqBody != nil {
		body, err = json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
	}

	target := c.endpoint + path
	req, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Inject headers
	// TODO: insert real headers
	if body != nil {
		req.Header.Add("Content-Type", "application/json;charset=utf-8")
	}
	req.Header.Add("Accept", "application/json")

	// Inject signature. Some methods do not need authentication, especially /time,
	// /auth and some /order methods are actually broken if authenticated.
	if needAuth {
		// TODO: insert auth process
	}

	// Send the request with requested timeout
	c.httpClient.Timeout = c.timeout

	return req, nil
}

// Do sends an HTTP request and returns an HTTP response
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c.logger != nil {
		c.logger.LogRequest(req)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if c.logger != nil {
		c.logger.LogResponse(resp)
	}
	return resp, nil
}


// UnmarshalResponse checks the response and unmarshals it into the response
// type if needed Helper function, called from CallAPI
func (c *Client) UnmarshalResponse(response *http.Response, resType interface{}) error {
	// Read all the response body
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	// < 200 && >= 300 then generate API error
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		// TODO: decide how to handle errors
		// apiError := &APIError{Code: response.StatusCode}
		// if err = json.Unmarshal(body, apiError); err != nil {
		// 	apiError.Message = string(body)
		// }
		// apiError.QueryID = response.Header.Get("X-Ovh-QueryID")

		// return apiError
	}

	// Nothing to unmarshal
	if len(body) == 0 || resType == nil {
		return nil
	}

	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	return d.Decode(&resType)
}