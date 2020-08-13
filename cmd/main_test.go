package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MyClient struct {
	mock.Mock
}

func (m *MyClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return (args.Get(0).(*http.Response)), (args.Error(1))
}

func TestRunMethodShouldSucceedIfNMIAnswerActive(t *testing.T) {
	myClientMock := new(MyClient)
	os.Setenv("HOST_IP", "192.168.1.1")
	r := ioutil.NopCloser(bytes.NewReader([]byte("Active")))

	myClientMock.On("Do", mock.Anything).Return(&http.Response{
		StatusCode: 200,
		Body:       r,
	}, nil)

	err := run(myClientMock)
	assert.Nil(t, err)
}

func TestRunMethodShouldFailIfNMIAnswerNotActive(t *testing.T) {
	myClientMock := new(MyClient)
	os.Setenv("HOST_IP", "192.168.1.1")
	r := ioutil.NopCloser(bytes.NewReader([]byte("Not Active")))

	myClientMock.On("Do", mock.Anything).Return(&http.Response{
		StatusCode: 200,
		Body:       r,
	}, nil)

	err := run(myClientMock)
	assert.Error(t, err)
	var e *NMIResponseWasNotActiveError
	assert.True(t, errors.As(err, &e))
}

func TestRunMethodShouldFailIfErrorCodeIsNotSet(t *testing.T) {
	myClientMock := new(MyClient)

	myClientMock.On("Do", mock.Anything).Return(&http.Response{
		StatusCode: 400,
	}, nil)

	err := run(myClientMock)
	assert.Error(t, err)
	var e *HostIPNotSetError
	assert.True(t, errors.As(err, &e))
}
