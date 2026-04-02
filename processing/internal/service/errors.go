package service

// ServiceError represents a typed service-layer error with an HTTP status code and error code.
// It is used by AuthService, OrgService, and APIKeyService.
type ServiceError struct {
	Code    string
	Message string
	Status  int
}

func (e *ServiceError) Error() string { return e.Message }
