// Code generated by MockGen. DO NOT EDIT.
// Source: client/lock_client.go

// Package mock_client is a generated GoMock package.
package mock_client

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockLockClient is a mock of LockClient interface
type MockLockClient struct {
	ctrl     *gomock.Controller
	recorder *MockLockClientMockRecorder
}

// MockLockClientMockRecorder is the mock recorder for MockLockClient
type MockLockClientMockRecorder struct {
	mock *MockLockClient
}

// NewMockLockClient creates a new mock instance
func NewMockLockClient(ctrl *gomock.Controller) *MockLockClient {
	mock := &MockLockClient{ctrl: ctrl}
	mock.recorder = &MockLockClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockLockClient) EXPECT() *MockLockClientMockRecorder {
	return m.recorder
}

// CreateLockRequest mocks base method
func (m *MockLockClient) CreateLockRequest() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateLockRequest")
	ret0, _ := ret[0].(bool)
	return ret0
}

// CreateLockRequest indicates an expected call of CreateLockRequest
func (mr *MockLockClientMockRecorder) CreateLockRequest() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateLockRequest", reflect.TypeOf((*MockLockClient)(nil).CreateLockRequest))
}

// CreateReleaseRequest mocks base method
func (m *MockLockClient) CreateReleaseRequest() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateReleaseRequest")
	ret0, _ := ret[0].(bool)
	return ret0
}

// CreateReleaseRequest indicates an expected call of CreateReleaseRequest
func (mr *MockLockClientMockRecorder) CreateReleaseRequest() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateReleaseRequest", reflect.TypeOf((*MockLockClient)(nil).CreateReleaseRequest))
}