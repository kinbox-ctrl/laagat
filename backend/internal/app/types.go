package app

import (
	"encoding/json"
	"time"
)

type User struct {
	ID        int64     `db:"id" json:"id"`
	Email     string    `db:"email" json:"email"`
	Name      string    `db:"name" json:"name"`
	Hash      string    `db:"password_hash" json:"-"`
	Role      string    `db:"role" json:"role"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

func (u *User) Staff() bool { return u.Role == "analyst" || u.Role == "admin" }

type Project struct {
	ID            int64      `db:"id" json:"id"`
	CustomerID    int64      `db:"customer_id" json:"customer_id"`
	CustomerName  string     `db:"customer_name" json:"customer_name"`
	CustomerEmail string     `db:"customer_email" json:"customer_email"`
	Title         string     `db:"title" json:"title"`
	City          string     `db:"city" json:"city"`
	State         string     `db:"state" json:"state"`
	BuildingType  string     `db:"building_type" json:"building_type"`
	AreaSqft      float64    `db:"built_up_area_sqft" json:"built_up_area_sqft"`
	Floors        string     `db:"floors" json:"floors"`
	Service       string     `db:"service" json:"service"`
	Status        string     `db:"status" json:"status"`
	Notes         string     `db:"notes" json:"notes"`
	QuotedTotal   float64    `db:"quoted_total" json:"quoted_total"`
	FeeAmount     float64    `db:"fee_amount" json:"fee_amount"`
	GSTAmount     float64    `db:"gst_amount" json:"gst_amount"`
	DeliveryDays  int        `db:"delivery_days" json:"delivery_days"`
	QuoteNote     string     `db:"quote_note" json:"quote_note"`
	PaymentRef    string     `db:"payment_ref" json:"payment_ref"`
	InvoiceNo     string     `db:"invoice_no" json:"invoice_no"`
	PaidAt        *time.Time `db:"paid_at" json:"paid_at"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
}

const projectSelect = `SELECT p.*, u.name AS customer_name, u.email AS customer_email FROM projects p JOIN users u ON u.id = p.customer_id`

type Document struct {
	ID         int64     `db:"id" json:"id"`
	ProjectID  int64     `db:"project_id" json:"project_id"`
	Kind       string    `db:"kind" json:"kind"`
	Filename   string    `db:"filename" json:"filename"`
	StorageKey string    `db:"storage_key" json:"-"`
	Size       int64     `db:"size" json:"size"`
	Revision   int       `db:"revision" json:"revision"`
	UploadedBy int64     `db:"uploaded_by" json:"uploaded_by"`
	Status     string    `db:"status" json:"status"`
	Note       string    `db:"note" json:"note"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

type BOQItem struct {
	ID            int64   `db:"id" json:"id"`
	ProjectID     int64   `db:"project_id" json:"project_id"`
	DocumentID    *int64  `db:"document_id" json:"document_id"`
	SlNo          string  `db:"sl_no" json:"sl_no"`
	Description   string  `db:"description" json:"description"`
	Unit          string  `db:"unit" json:"unit"`
	Quantity      float64 `db:"quantity" json:"quantity"`
	Rate          float64 `db:"rate" json:"rate"`
	Amount        float64 `db:"amount" json:"amount"`
	WorkItem      string  `db:"work_item" json:"work_item"`
	MappingSource string  `db:"mapping_source" json:"mapping_source"`
	AISuggestion  string  `db:"ai_suggestion" json:"ai_suggestion"`
	Sort          int     `db:"sort" json:"sort"`
}

type Measurement struct {
	ID          int64     `db:"id" json:"id"`
	ProjectID   int64     `db:"project_id" json:"project_id"`
	WorkItem    string    `db:"work_item" json:"work_item"`
	Description string    `db:"description" json:"description"`
	Method      string    `db:"method" json:"method"`
	Nos         float64   `db:"nos" json:"nos"`
	Length      float64   `db:"length" json:"length"`
	Breadth     float64   `db:"breadth" json:"breadth"`
	Depth       float64   `db:"depth" json:"depth"`
	DiaMM       float64   `db:"dia_mm" json:"dia_mm"`
	IsDeduction bool      `db:"is_deduction" json:"is_deduction"`
	Quantity    float64   `db:"quantity" json:"quantity"`
	Formula     string    `db:"formula" json:"formula"`
	DocumentID  *int64    `db:"document_id" json:"document_id"`
	Remarks     string    `db:"remarks" json:"remarks"`
	CreatedBy   int64     `db:"created_by" json:"created_by"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

type Assumption struct {
	ID        int64  `db:"id" json:"id"`
	ProjectID int64  `db:"project_id" json:"project_id"`
	Kind      string `db:"kind" json:"kind"`
	Body      string `db:"body" json:"body"`
}

type FindingRow struct {
	ID          int64           `db:"id" json:"id"`
	ProjectID   int64           `db:"project_id" json:"project_id"`
	Fingerprint string          `db:"fingerprint" json:"fingerprint"`
	Code        string          `db:"code" json:"code"`
	Severity    string          `db:"severity" json:"severity"`
	Title       string          `db:"title" json:"title"`
	Explanation string          `db:"explanation" json:"explanation"`
	Action      string          `db:"required_action" json:"required_action"`
	Evidence    json.RawMessage `db:"evidence" json:"evidence"`
	Impact      float64         `db:"amount_impact" json:"amount_impact"`
	BOQItemID   *int64          `db:"boq_item_id" json:"boq_item_id"`
	Status      string          `db:"status" json:"status"`
	AnalystNote string          `db:"analyst_note" json:"analyst_note"`
	AIDraft     string          `db:"ai_draft" json:"ai_draft"`
	ReviewedBy  *int64          `db:"reviewed_by" json:"reviewed_by"`
}

type Report struct {
	ID        int64     `db:"id" json:"id"`
	ProjectID int64     `db:"project_id" json:"project_id"`
	Version   int       `db:"version" json:"version"`
	Approver  string    `db:"approver" json:"approved_by"`
	PDFKey    string    `db:"pdf_key" json:"-"`
	XLSXKey   string    `db:"xlsx_key" json:"-"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Message struct {
	ID        int64     `db:"id" json:"id"`
	UserID    int64     `db:"user_id" json:"user_id"`
	Author    string    `db:"author" json:"author"`
	Role      string    `db:"role" json:"role"`
	Body      string    `db:"body" json:"body"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Rate struct {
	ID            int64   `db:"id" json:"id"`
	Resource      string  `db:"resource" json:"resource"`
	Location      string  `db:"location" json:"location"`
	Rate          float64 `db:"rate" json:"rate"`
	EffectiveDate string  `db:"effective_date" json:"effective_date"`
	Source        string  `db:"source" json:"source"`
	Spec          string  `db:"spec" json:"spec"`
	InclTax       bool    `db:"incl_tax" json:"incl_tax"`
	Verified      bool    `db:"verified" json:"verified"`
}

const rateSelect = `SELECT id, resource, location, rate, to_char(effective_date,'YYYY-MM-DD') AS effective_date, source, spec, incl_tax, verified FROM rates`
