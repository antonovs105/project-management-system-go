package comment

import "time"

type Comment struct {
	ID        string    `db:"id" json:"id"`
	APID      string    `db:"ap_id" json:"ap_id"`
	TicketID  string    `db:"ticket_id" json:"ticket_id"`
	AuthorID  string    `db:"author_id" json:"author_id"`
	Content   string    `db:"content" json:"content"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
