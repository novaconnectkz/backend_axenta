DO $$
DECLARE
    tenant RECORD;
BEGIN
    FOR tenant IN
        SELECT schema_name
        FROM information_schema.schemata
        WHERE schema_name LIKE 'tenant_%'
    LOOP
        EXECUTE format('ALTER TABLE %I.objects DROP CONSTRAINT IF EXISTS fk_contracts_objects', tenant.schema_name);
        EXECUTE format('ALTER TABLE %I.objects DROP CONSTRAINT IF EXISTS fk_locations_objects', tenant.schema_name);
        EXECUTE format('ALTER TABLE %I.objects ALTER COLUMN contract_id DROP NOT NULL', tenant.schema_name);
        EXECUTE format('ALTER TABLE %I.objects ALTER COLUMN contract_id SET DEFAULT 0', tenant.schema_name);
        EXECUTE format('ALTER TABLE %I.objects ALTER COLUMN location_id SET DEFAULT 0', tenant.schema_name);
        EXECUTE format('UPDATE %I.objects SET contract_id = COALESCE(contract_id, 0)', tenant.schema_name);
        EXECUTE format('UPDATE %I.objects SET location_id = COALESCE(location_id, 0)', tenant.schema_name);
    END LOOP;
END $$;

