DROP TABLE IF EXISTS invitations;

DROP TRIGGER IF EXISTS trg_audit_logs_no_delete ON audit_logs;
DROP TRIGGER IF EXISTS trg_audit_logs_no_update ON audit_logs;
DROP FUNCTION IF EXISTS prevent_audit_log_mutation();
DROP TABLE IF EXISTS audit_logs;

DROP TABLE IF EXISTS targeting_rules;
DROP TYPE IF EXISTS targeting_action;
DROP TYPE IF EXISTS targeting_operator;

DROP TABLE IF EXISTS flag_environments;

DROP TABLE IF EXISTS flags;
DROP TYPE IF EXISTS flag_type;

DROP TABLE IF EXISTS environments;

DROP TABLE IF EXISTS refresh_tokens;

DROP TABLE IF EXISTS memberships;
DROP TYPE IF EXISTS membership_role;

DROP TABLE IF EXISTS users;

DROP TABLE IF EXISTS organizations;