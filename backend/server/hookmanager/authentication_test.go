package hookmanager

import (
	"context"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"isc.org/stork/hooks"
	"isc.org/stork/hooks/server/authenticationcallouts"
)

// Carrier mock interface for mockgen.
type authenticationCalloutCarrier interface { //nolint:unused
	authenticationcallouts.AuthenticationCallouts
	hooks.CalloutCarrier
}

//go:generate mockgen -package=hookmanager -destination=hookmanager_mock.go -source=authentication_test.go -mock_names=authenticationCalloutCarrier=MockAuthenticationCalloutCarrier isc.org/server/hookmanager authenticationCalloutCarrier

// Test that the authentication callout is called.
func TestAuthenticate(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	username := "foo"
	password := "bar"

	mock := NewMockAuthenticationCalloutCarrier(ctrl)
	mock.EXPECT().
		Authenticate(gomock.Any(), gomock.Any(), &username, &password).
		Return(&authenticationcallouts.User{
			ID:       42,
			Login:    "foo",
			Email:    "foo@example.com",
			Lastname: "oof",
			Name:     "ofo",
			Groups:   []int{1, 2, 3},
		}, nil).
		Times(1)

	hookManager := NewHookManager()
	hookManager.RegisterCalloutCarriers([]hooks.CalloutCarrier{mock})

	// Act
	user, err := hookManager.Authenticate(context.Background(), nil, "default", &username, &password)

	// Assert
	require.NoError(t, err)
	require.EqualValues(t, "foo@example.com", user.Email)
}

// Test that only first authentication callout is called.
func TestAuthenticateIsSingle(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock1 := NewMockAuthenticationCalloutCarrier(ctrl)
	mock1.EXPECT().
		Authenticate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&authenticationcallouts.User{}, nil).
		Times(1)

	mock2 := NewMockAuthenticationCalloutCarrier(ctrl)
	mock2.EXPECT().
		Authenticate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&authenticationcallouts.User{}, nil).
		Times(0)

	hookManager := NewHookManager()
	hookManager.RegisterCalloutCarriers([]hooks.CalloutCarrier{mock1, mock2})

	// Act
	user, err := hookManager.Authenticate(context.Background(), nil, "", nil, nil)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, user)
}

// Test that the error is returned if the authentication fails.
func TestAuthenticateReturnError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock1 := NewMockAuthenticationCalloutCarrier(ctrl)
	mock1.EXPECT().
		Authenticate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("foo")).
		Times(1)

	mock2 := NewMockAuthenticationCalloutCarrier(ctrl)
	mock2.EXPECT().
		Authenticate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("bar")).
		Times(0)

	hookManager := NewHookManager()
	hookManager.RegisterCalloutCarriers([]hooks.CalloutCarrier{mock1, mock2})

	// Act
	user, err := hookManager.Authenticate(context.Background(), nil, "", nil, nil)

	// Assert
	require.ErrorContains(t, err, "foo")
	require.Nil(t, user)
}

// Test that the authentication callout returns a default value if no callouts
// are registered.
func TestAuthenticateDefault(t *testing.T) {
	// Arrange
	hookManager := NewHookManager()

	// Act
	user, err := hookManager.Authenticate(context.Background(), nil, "", nil, nil)

	// Assert
	require.NoError(t, err)
	require.Nil(t, user)
}

// Test that the unauthenticate function is called only once.
func TestUnauthenticate(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock1 := NewMockAuthenticationCalloutCarrier(ctrl)
	mock1.EXPECT().
		Unauthenticate(gomock.Any()).
		Return(nil).
		Times(1)

	mock2 := NewMockAuthenticationCalloutCarrier(ctrl)
	mock2.EXPECT().
		Unauthenticate(gomock.Any()).
		Return(nil).
		Times(0)

	hookManager := NewHookManager()
	hookManager.RegisterCalloutCarriers([]hooks.CalloutCarrier{mock1, mock2})

	// Act
	err := hookManager.Unauthenticate(context.Background())

	// Assert
	require.NoError(t, err)
}

// Test that the unauthenticate function returns an error properly.
func TestUnauthenticateError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock1 := NewMockAuthenticationCalloutCarrier(ctrl)
	mock1.EXPECT().
		Unauthenticate(gomock.Any()).
		Return(errors.New("foo")).
		Times(1)

	mock2 := NewMockAuthenticationCalloutCarrier(ctrl)
	mock2.EXPECT().
		Unauthenticate(gomock.Any()).
		Return(errors.New("bar")).
		Times(0)

	hookManager := NewHookManager()
	hookManager.RegisterCalloutCarriers([]hooks.CalloutCarrier{mock1, mock2})

	// Act
	err := hookManager.Unauthenticate(context.Background())

	// Assert
	require.ErrorContains(t, err, "foo")
}
