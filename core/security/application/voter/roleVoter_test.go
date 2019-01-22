package voter

import (
	"context"
	"testing"

	roleMocks "flamingo.me/flamingo/v3/core/security/application/role/mocks"
	"flamingo.me/flamingo/v3/core/security/domain"
	domainMocks "flamingo.me/flamingo/v3/core/security/domain/mocks"
	"flamingo.me/flamingo/v3/framework/web"
	"github.com/stretchr/testify/suite"
)

type (
	RoleVoterTestSuite struct {
		suite.Suite

		voter       *RoleVoter
		roleService *roleMocks.Service
		object      *domainMocks.RoleSet

		context    context.Context
		webSession *web.Session
	}
)

func TestRoleVoterTestSuite(t *testing.T) {
	suite.Run(t, &RoleVoterTestSuite{})
}

func (t *RoleVoterTestSuite) SetupSuite() {
	t.context = context.Background()
	t.webSession = web.EmptySession()
}

func (t *RoleVoterTestSuite) SetupTest() {
	t.roleService = &roleMocks.Service{}
	t.object = &domainMocks.RoleSet{}
	t.voter = &RoleVoter{}
	t.voter.Inject(t.roleService)
}

func (t *RoleVoterTestSuite) TearDownTest() {
	t.roleService.AssertExpectations(t.T())
	t.roleService = nil
	t.object.AssertExpectations(t.T())
	t.object = nil
	t.voter = nil
}

func (t *RoleVoterTestSuite) TestVote_AccessAbstained() {
	t.Equal(AccessAbstained, t.voter.Vote(t.context, t.webSession, domain.RoleUser.Permission(), nil))
}

func (t *RoleVoterTestSuite) TestVote_AccessGrantedWithoutObject() {
	t.roleService.On("All", t.context, t.webSession).Return([]domain.Role{
		domain.RoleUser,
		domain.Role("RoleAdministrator"),
	}).Once()
	t.Equal(AccessGranted, t.voter.Vote(t.context, t.webSession, "RoleAdministrator", nil))
}

func (t *RoleVoterTestSuite) TestVote_AccessGrantedWithObject() {
	t.roleService.On("All", t.context, t.webSession).Return([]domain.Role{
		domain.RoleUser,
		domain.Role("RoleAdministrator"),
	}).Once()
	t.object.On("Roles").Return([]domain.Role{
		domain.Role("RoleAdministrator"),
	}).Once()
	t.Equal(AccessGranted, t.voter.Vote(t.context, t.webSession, "RoleAdministrator", t.object))
}

func (t *RoleVoterTestSuite) TestVote_AccessDeniedWithoutObject() {
	t.roleService.On("All", t.context, t.webSession).Return([]domain.Role{}).Once()
	t.Equal(AccessDenied, t.voter.Vote(t.context, t.webSession, "RoleAdministrator", nil))
}

func (t *RoleVoterTestSuite) TestVote_AccessDeniedWithObject() {
	t.object.On("Roles").Return([]domain.Role{}).Once()
	t.Equal(AccessDenied, t.voter.Vote(t.context, t.webSession, "RoleAdministrator", t.object))
}
