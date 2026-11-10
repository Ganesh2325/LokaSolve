CREATE TABLE categories (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT categories_name_unique UNIQUE (name),
  CONSTRAINT categories_name_len CHECK (char_length(name) BETWEEN 2 AND 80)
);

CREATE TABLE problem_types (
  id uuid PRIMARY KEY,
  category_id uuid NOT NULL REFERENCES categories (id),
  name text NOT NULL,
  prompt text,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT problem_types_name_unique UNIQUE (category_id, name),
  CONSTRAINT problem_types_name_len CHECK (char_length(name) BETWEEN 2 AND 80)
);

ALTER TABLE issues ADD COLUMN problem_type_id uuid REFERENCES problem_types (id);

ALTER TABLE categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE categories FORCE ROW LEVEL SECURITY;
ALTER TABLE problem_types ENABLE ROW LEVEL SECURITY;
ALTER TABLE problem_types FORCE ROW LEVEL SECURITY;

CREATE POLICY categories_select ON categories FOR SELECT USING (true);
CREATE POLICY categories_write ON categories FOR ALL
  USING (app_flag('app.platform_admin'))
  WITH CHECK (app_flag('app.platform_admin'));

CREATE POLICY problem_types_select ON problem_types FOR SELECT USING (true);
CREATE POLICY problem_types_write ON problem_types FOR ALL
  USING (app_flag('app.platform_admin'))
  WITH CHECK (app_flag('app.platform_admin'));

GRANT SELECT, INSERT, UPDATE, DELETE ON categories TO lokasolve_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON problem_types TO lokasolve_app;
