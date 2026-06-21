package comment

import (
	"context"
	"errors"
	"testing"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	apdelivery "github.com/antonovs105/project-management-system-go/internal/activitypub/delivery"
	"github.com/antonovs105/project-management-system-go/internal/project"
	"github.com/antonovs105/project-management-system-go/internal/ticket"
	"github.com/stretchr/testify/require"
)

func TestServiceCreateCommentWritesNoteAndQueuesRecipients(t *testing.T) {
	ctx := context.Background()
	cfg := activitypub.NewConfig("http://localhost:8080", "localhost:8080")
	repo := &fakeCommentRepository{
		createResult: &CreateResult{
			ActivityID: "activity-create",
			ProjectID:  "project-1",
			TicketID:   "ticket-1",
			Deliveries: []apdelivery.QueueCandidate{{ID: "delivery-create", MaxAttempts: 10}},
		},
	}
	tickets := &fakeTicketChecker{
		ticket:      &ticket.Ticket{ID: "ticket-1", ProjectID: "project-1"},
		permissions: map[string]bool{"project-1:user-1:" + project.PermissionCommentsCreate: true},
	}
	delivery := &fakeCommentDelivery{}
	service := NewService(repo, tickets, cfg)
	service.SetDelivery(delivery)

	created, err := service.CreateComment(ctx, "ticket-1", "user-1", "  Hello ActivityPub  ")

	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, "Hello ActivityPub", created.Content)
	require.Len(t, repo.created, 1)
	require.Equal(t, repo.created[0].ID, created.ID)
	require.Equal(t, activitypub.CommentAPID(cfg, created.ID), created.APID)
	require.Equal(t, "ticket-1", repo.created[0].TicketID)
	require.Equal(t, "user-1", repo.created[0].AuthorID)
	require.Equal(t, []apdelivery.QueueCandidate{{ID: "delivery-create", MaxAttempts: 10}}, delivery.deliveries)
}

func TestServiceCreateCommentRejectsViewer(t *testing.T) {
	ctx := context.Background()
	repo := &fakeCommentRepository{createResult: &CreateResult{ActivityID: "activity-create"}}
	tickets := &fakeTicketChecker{
		ticket:      &ticket.Ticket{ID: "ticket-1", ProjectID: "project-1"},
		permissions: map[string]bool{},
	}
	service := NewService(repo, tickets, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	created, err := service.CreateComment(ctx, "ticket-1", "viewer-1", "Nope")

	require.Error(t, err)
	require.Nil(t, created)
	require.Contains(t, err.Error(), "comments.create")
	require.Empty(t, repo.created)
}

func TestServiceListCommentsRequiresTicketAccess(t *testing.T) {
	ctx := context.Background()
	repo := &fakeCommentRepository{list: []Comment{{ID: "comment-1"}}}
	tickets := &fakeTicketChecker{ticketErr: errors.New("ticket not found")}
	service := NewService(repo, tickets, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))

	comments, err := service.ListComments(ctx, "ticket-1", "user-1", CommentListOptions{})

	require.Error(t, err)
	require.Nil(t, comments)
	require.Empty(t, repo.listTicketID)
}

func TestServiceDeleteCommentAllowsManagerToModerateAndQueuesInboxes(t *testing.T) {
	ctx := context.Background()
	repo := &fakeCommentRepository{
		commentsByID: map[string]*Comment{
			"comment-1": {ID: "comment-1", TicketID: "ticket-1", AuthorID: "author-1"},
		},
		deleteResult: &DeleteResult{
			ActivityID:       "activity-delete",
			ProjectID:        "project-1",
			RecipientInboxes: []string{"https://remote.example/users/alice/inbox", ""},
			Deliveries:       []apdelivery.QueueCandidate{{ID: "delivery-delete", MaxAttempts: 10}},
		},
	}
	tickets := &fakeTicketChecker{
		ticket:      &ticket.Ticket{ID: "ticket-1", ProjectID: "project-1"},
		permissions: map[string]bool{"project-1:manager-1:" + project.PermissionCommentsModerate: true},
	}
	delivery := &fakeCommentDelivery{}
	service := NewService(repo, tickets, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	service.SetDelivery(delivery)

	err := service.DeleteComment(ctx, "comment-1", "manager-1")

	require.NoError(t, err)
	require.Equal(t, "comment-1", repo.deletedCommentID)
	require.Equal(t, "manager-1", repo.deletedActorID)
	require.Equal(t, []apdelivery.QueueCandidate{{ID: "delivery-delete", MaxAttempts: 10}}, delivery.deliveries)
}

func TestServiceDeleteCommentRejectsOtherDeveloper(t *testing.T) {
	ctx := context.Background()
	repo := &fakeCommentRepository{
		commentsByID: map[string]*Comment{
			"comment-1": {ID: "comment-1", TicketID: "ticket-1", AuthorID: "author-1"},
		},
		deleteResult: &DeleteResult{ActivityID: "activity-delete", ProjectID: "project-1"},
	}
	tickets := &fakeTicketChecker{
		ticket:      &ticket.Ticket{ID: "ticket-1", ProjectID: "project-1"},
		permissions: map[string]bool{"project-1:dev-1:" + project.PermissionCommentsCreate: true},
	}
	delivery := &fakeCommentDelivery{}
	service := NewService(repo, tickets, activitypub.NewConfig("http://localhost:8080", "localhost:8080"))
	service.SetDelivery(delivery)

	err := service.DeleteComment(ctx, "comment-1", "dev-1")

	require.Error(t, err)
	require.Contains(t, err.Error(), "comments.moderate")
	require.Empty(t, repo.deletedCommentID)
	require.Empty(t, delivery.deliveries)
}

type fakeCommentRepository struct {
	createResult *CreateResult
	createErr    error
	created      []Comment

	commentsByID map[string]*Comment
	getErr       error

	list         []Comment
	listErr      error
	listTicketID string

	deleteResult     *DeleteResult
	deleteErr        error
	deletedCommentID string
	deletedActorID   string
}

func (f *fakeCommentRepository) Create(ctx context.Context, comment *Comment) (*CreateResult, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = append(f.created, *comment)
	return f.createResult, nil
}

func (f *fakeCommentRepository) GetByID(ctx context.Context, commentID string) (*Comment, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	comment, ok := f.commentsByID[commentID]
	if !ok {
		return nil, errors.New("comment not found")
	}
	copied := *comment
	return &copied, nil
}

func (f *fakeCommentRepository) ListByTicketID(ctx context.Context, ticketID string, options CommentListOptions) ([]Comment, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	f.listTicketID = ticketID
	return f.list, nil
}

func (f *fakeCommentRepository) Delete(ctx context.Context, commentID string, actorID string) (*DeleteResult, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	f.deletedCommentID = commentID
	f.deletedActorID = actorID
	return f.deleteResult, nil
}

type fakeTicketChecker struct {
	ticket      *ticket.Ticket
	ticketErr   error
	permissions map[string]bool
	roleErr     error
}

func (f *fakeTicketChecker) GetTicketByID(ctx context.Context, ticketID, userID string) (*ticket.Ticket, error) {
	if f.ticketErr != nil {
		return nil, f.ticketErr
	}
	copied := *f.ticket
	return &copied, nil
}

func (f *fakeTicketChecker) HasProjectPermission(ctx context.Context, projectID, userID, permission string) (bool, error) {
	if f.roleErr != nil {
		return false, f.roleErr
	}
	return f.permissions[projectID+":"+userID+":"+permission], nil
}

type fakeCommentDelivery struct {
	deliveries []apdelivery.QueueCandidate
}

func (f *fakeCommentDelivery) EnqueuePersisted(ctx context.Context, deliveries []apdelivery.QueueCandidate) error {
	f.deliveries = append(f.deliveries, deliveries...)
	return nil
}
