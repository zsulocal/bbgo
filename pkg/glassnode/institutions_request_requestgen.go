// Code generated by "requestgen -method GET -type InstitutionsRequest -url /v1/metrics/institutions/:metric -responseType Response"; DO NOT EDIT.

package glassnode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
)

func (i *InstitutionsRequest) SetAsset(Asset string) *InstitutionsRequest {
	i.Asset = Asset
	return i
}

func (i *InstitutionsRequest) SetSince(Since int64) *InstitutionsRequest {
	i.Since = Since
	return i
}

func (i *InstitutionsRequest) SetUntil(Until int64) *InstitutionsRequest {
	i.Until = Until
	return i
}

func (i *InstitutionsRequest) SetInterval(Interval Interval) *InstitutionsRequest {
	i.Interval = Interval
	return i
}

func (i *InstitutionsRequest) SetFormat(Format Format) *InstitutionsRequest {
	i.Format = Format
	return i
}

func (i *InstitutionsRequest) SetTimestampFormat(TimestampFormat string) *InstitutionsRequest {
	i.TimestampFormat = TimestampFormat
	return i
}

func (i *InstitutionsRequest) SetMetric(Metric string) *InstitutionsRequest {
	i.Metric = Metric
	return i
}

// GetQueryParameters builds and checks the query parameters and returns url.Values
func (i *InstitutionsRequest) GetQueryParameters() (url.Values, error) {
	var params = map[string]interface{}{}
	// check Asset field -> json key a
	Asset := i.Asset

	// TEMPLATE check-required
	if len(Asset) == 0 {
		return nil, fmt.Errorf("a is required, empty string given")
	}
	// END TEMPLATE check-required

	// assign parameter of Asset
	params["a"] = Asset
	// check Since field -> json key s
	Since := i.Since

	// assign parameter of Since
	params["s"] = Since
	// check Until field -> json key u
	Until := i.Until

	// assign parameter of Until
	params["u"] = Until
	// check Interval field -> json key i
	Interval := i.Interval

	// assign parameter of Interval
	params["i"] = Interval
	// check Format field -> json key f
	Format := i.Format

	// assign parameter of Format
	params["f"] = Format
	// check TimestampFormat field -> json key timestamp_format
	TimestampFormat := i.TimestampFormat

	// assign parameter of TimestampFormat
	params["timestamp_format"] = TimestampFormat

	query := url.Values{}
	for k, v := range params {
		query.Add(k, fmt.Sprintf("%v", v))
	}

	return query, nil
}

// GetParameters builds and checks the parameters and return the result in a map object
func (i *InstitutionsRequest) GetParameters() (map[string]interface{}, error) {
	var params = map[string]interface{}{}

	return params, nil
}

// GetParametersQuery converts the parameters from GetParameters into the url.Values format
func (i *InstitutionsRequest) GetParametersQuery() (url.Values, error) {
	query := url.Values{}

	params, err := i.GetParameters()
	if err != nil {
		return query, err
	}

	for k, v := range params {
		query.Add(k, fmt.Sprintf("%v", v))
	}

	return query, nil
}

// GetParametersJSON converts the parameters from GetParameters into the JSON format
func (i *InstitutionsRequest) GetParametersJSON() ([]byte, error) {
	params, err := i.GetParameters()
	if err != nil {
		return nil, err
	}

	return json.Marshal(params)
}

// GetSlugParameters builds and checks the slug parameters and return the result in a map object
func (i *InstitutionsRequest) GetSlugParameters() (map[string]interface{}, error) {
	var params = map[string]interface{}{}
	// check Metric field -> json key metric
	Metric := i.Metric

	// assign parameter of Metric
	params["metric"] = Metric

	return params, nil
}

func (i *InstitutionsRequest) applySlugsToUrl(url string, slugs map[string]string) string {
	for k, v := range slugs {
		needleRE := regexp.MustCompile(":" + k + "\\b")
		url = needleRE.ReplaceAllString(url, v)
	}

	return url
}

func (i *InstitutionsRequest) GetSlugsMap() (map[string]string, error) {
	slugs := map[string]string{}
	params, err := i.GetSlugParameters()
	if err != nil {
		return slugs, nil
	}

	for k, v := range params {
		slugs[k] = fmt.Sprintf("%v", v)
	}

	return slugs, nil
}

func (i *InstitutionsRequest) Do(ctx context.Context) (Response, error) {

	// no body params
	var params interface{}
	query, err := i.GetQueryParameters()
	if err != nil {
		return nil, err
	}

	apiURL := "/v1/metrics/institutions/:metric"
	slugs, err := i.GetSlugsMap()
	if err != nil {
		return nil, err
	}

	apiURL = i.applySlugsToUrl(apiURL, slugs)

	req, err := i.Client.NewAuthenticatedRequest(ctx, "GET", apiURL, query, params)
	if err != nil {
		return nil, err
	}

	response, err := i.Client.SendRequest(req)
	if err != nil {
		return nil, err
	}

	var apiResponse Response
	if err := response.DecodeJSON(&apiResponse); err != nil {
		return nil, err
	}
	return apiResponse, nil
}
