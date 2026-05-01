DO $$
DECLARE
    tenant RECORD;
BEGIN
    FOR tenant IN
        SELECT schema_name
        FROM information_schema.schemata
        WHERE schema_name LIKE 'tenant_%'
    LOOP
        EXECUTE format('INSERT INTO %I.companies (id, created_at, updated_at, name, database_schema, axetna_login, axetna_password) VALUES (6, NOW(), NOW(), $1, $2, $3, $4) ON CONFLICT (id) DO NOTHING', tenant.schema_name)
        USING 'Default Company', tenant.schema_name, 'glomos', 'placeholder';
    END LOOP;
END $$;

