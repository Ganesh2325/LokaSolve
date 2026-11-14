package taxonomy

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/events"
	"lokasolve/services/api-go/internal/platform/audit"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
)

type Service struct {
	DB *database.DB
}

type FieldDef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type ProblemType struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Prompt *string    `json:"prompt"`
	Fields []FieldDef `json:"fields"`
}

type Category struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	ProblemTypes []ProblemType `json:"problem_types"`
}

func (s *Service) List(ctx context.Context, principal authn.Principal) ([]Category, error) {
	categories := []Category{}
	err := s.DB.WithFlags(ctx, &principal.UserID, nil, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT c.id::text, c.name, p.id::text, p.name, p.prompt, p.fields::text
			FROM categories c
			LEFT JOIN problem_types p ON p.category_id = c.id AND p.active
			WHERE c.active
			ORDER BY c.name, p.name
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		index := map[string]int{}
		for rows.Next() {
			var categoryID, categoryName string
			var typeID, typeName, prompt, rawFields *string
			if err := rows.Scan(&categoryID, &categoryName, &typeID, &typeName, &prompt, &rawFields); err != nil {
				return err
			}
			position, ok := index[categoryID]
			if !ok {
				categories = append(categories, Category{ID: categoryID, Name: categoryName, ProblemTypes: []ProblemType{}})
				position = len(categories) - 1
				index[categoryID] = position
			}
			if typeID != nil && typeName != nil {
				defs := []FieldDef{}
				if rawFields != nil && *rawFields != "" {
					_ = json.Unmarshal([]byte(*rawFields), &defs)
				}
				categories[position].ProblemTypes = append(categories[position].ProblemTypes, ProblemType{ID: *typeID, Name: *typeName, Prompt: prompt, Fields: defs})
			}
		}
		return rows.Err()
	})
	return categories, err
}

func (s *Service) AddProblemType(ctx context.Context, principal authn.Principal, categoryName, typeName, prompt, fieldLabel, requestID string) (ProblemType, error) {
	if !principal.Has("admin:taxonomy_manage") && !principal.Has("admin:roles_grant") {
		return ProblemType{}, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	categoryName = strings.TrimSpace(categoryName)
	typeName = strings.TrimSpace(typeName)
	prompt = strings.TrimSpace(prompt)
	fields := map[string]string{}
	if len([]rune(categoryName)) < 2 || len([]rune(categoryName)) > 80 {
		fields["category_name"] = "Use 2 to 80 characters."
	}
	if len([]rune(typeName)) < 2 || len([]rune(typeName)) > 80 {
		fields["name"] = "Use 2 to 80 characters."
	}
	if len(fields) > 0 {
		return ProblemType{}, httpx.Validation(fields)
	}
	var promptValue *string
	if prompt != "" {
		promptValue = &prompt
	}
	defs := []FieldDef{}
	if field := strings.TrimSpace(fieldLabel); field != "" {
		defs = append(defs, FieldDef{Key: "detail", Label: field})
	}
	rawFields, err := json.Marshal(defs)
	if err != nil {
		return ProblemType{}, err
	}
	categoryID := uuid.New()
	typeID := uuid.New()
	var created ProblemType
	err = s.DB.WithFlags(ctx, &principal.UserID, []string{"app.platform_admin"}, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO categories (id, name) VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE SET active = true
			RETURNING id
		`, categoryID, categoryName).Scan(&categoryID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO problem_types (id, category_id, name, prompt, fields) VALUES ($1, $2, $3, $4, $5::jsonb)
			ON CONFLICT (category_id, name) DO UPDATE SET active = true, prompt = EXCLUDED.prompt, fields = EXCLUDED.fields
			RETURNING id::text, name, prompt
		`, typeID, categoryID, typeName, promptValue, rawFields).Scan(&created.ID, &created.Name, &created.Prompt); err != nil {
			return err
		}
		id, _ := uuid.Parse(created.ID)
		created.Fields = defs
		if err := events.Enqueue(ctx, tx, "taxonomy", id, "taxonomy.updated", map[string]any{
			"problem_type_id": created.ID, "category_id": categoryID.String(), "name": created.Name,
		}); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, ActorRole: "operator", Action: "taxonomy.problem_type_upsert",
			ResourceType: "problem_type", ResourceID: &id, RequestID: requestID,
			Metadata: map[string]any{"category": categoryName, "name": typeName},
		})
	})
	return created, err
}
