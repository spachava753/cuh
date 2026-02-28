//go:build !darwin || !cgo

package contacts

func authorizationStatus() (AuthStatus, error) {
	return "", ErrUnsupportedPlatform
}

func requestAccess() error {
	return ErrUnsupportedPlatform
}

func find(input FindInput) (FindOutput, error) {
	_ = input
	return FindOutput{}, ErrUnsupportedPlatform
}

func get(input GetInput) (GetOutput, error) {
	_ = input
	return GetOutput{}, ErrUnsupportedPlatform
}

func upsert(input UpsertInput) (UpsertOutput, error) {
	_ = input
	return UpsertOutput{}, ErrUnsupportedPlatform
}

func mutate(input MutateInput) (MutateOutput, error) {
	_ = input
	return MutateOutput{}, ErrUnsupportedPlatform
}

func groups(input GroupsInput) (GroupsOutput, error) {
	_ = input
	return GroupsOutput{}, ErrUnsupportedPlatform
}
