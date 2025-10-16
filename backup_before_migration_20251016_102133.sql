--
-- PostgreSQL database dump
--

\restrict mKglDIRbBA4TZpqqCRInQiyXq7k0qVBquLvxUfQgaDzFudkT1pYxTfryxRh736n

-- Dumped from database version 14.19 (Homebrew)
-- Dumped by pg_dump version 14.19 (Homebrew)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: tenant_default; Type: SCHEMA; Schema: -; Owner: postgres
--

CREATE SCHEMA tenant_default;


ALTER SCHEMA tenant_default OWNER TO postgres;

--
-- Name: tenant_newacrm; Type: SCHEMA; Schema: -; Owner: postgres
--

CREATE SCHEMA tenant_newacrm;


ALTER SCHEMA tenant_newacrm OWNER TO postgres;

--
-- Name: tenant_yyqqqqqq; Type: SCHEMA; Schema: -; Owner: postgres
--

CREATE SCHEMA tenant_yyqqqqqq;


ALTER SCHEMA tenant_yyqqqqqq OWNER TO postgres;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: billing_plans; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.billing_plans (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    description text,
    price numeric(10,2) NOT NULL,
    currency character varying(3) DEFAULT 'RUB'::character varying,
    billing_period character varying(20) DEFAULT 'monthly'::character varying,
    max_devices bigint DEFAULT 0,
    max_users bigint DEFAULT 0,
    max_storage bigint DEFAULT 0,
    has_analytics boolean DEFAULT false,
    has_api boolean DEFAULT false,
    has_support boolean DEFAULT false,
    has_custom_domain boolean DEFAULT false,
    is_active boolean DEFAULT true,
    is_popular boolean DEFAULT false,
    company_id bigint
);


ALTER TABLE public.billing_plans OWNER TO postgres;

--
-- Name: billing_plans_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.billing_plans_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.billing_plans_id_seq OWNER TO postgres;

--
-- Name: billing_plans_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.billing_plans_id_seq OWNED BY public.billing_plans.id;


--
-- Name: companies; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.companies (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    database_schema character varying(100) NOT NULL,
    domain character varying(100),
    axetna_login character varying(100) NOT NULL,
    axetna_password text NOT NULL,
    bitrix24_webhook_url character varying(500),
    bitrix24_client_id character varying(100),
    bitrix24_client_secret character varying(200),
    contact_email character varying(100),
    contact_phone character varying(20),
    contact_person character varying(100),
    address text,
    city character varying(100),
    country character varying(100) DEFAULT 'Russia'::character varying,
    is_active boolean DEFAULT true,
    max_users bigint DEFAULT 10,
    max_objects bigint DEFAULT 100,
    storage_quota bigint DEFAULT 1024,
    language character varying(5) DEFAULT 'ru'::character varying,
    timezone character varying(50) DEFAULT 'Europe/Moscow'::character varying,
    currency character varying(3) DEFAULT 'RUB'::character varying,
    subscription_id bigint
);


ALTER TABLE public.companies OWNER TO postgres;

--
-- Name: companies_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.companies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.companies_id_seq OWNER TO postgres;

--
-- Name: companies_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.companies_id_seq OWNED BY public.companies.id;


--
-- Name: contract_appendices; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.contract_appendices (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    contract_id bigint NOT NULL,
    number character varying(50) NOT NULL,
    title character varying(200) NOT NULL,
    description text,
    start_date timestamp with time zone NOT NULL,
    end_date timestamp with time zone NOT NULL,
    signed_at timestamp with time zone,
    amount numeric(15,2),
    currency character varying(3) DEFAULT 'RUB'::character varying,
    status character varying(20) DEFAULT 'draft'::character varying,
    is_active boolean DEFAULT true,
    notes text,
    external_id character varying(100)
);


ALTER TABLE public.contract_appendices OWNER TO postgres;

--
-- Name: contract_appendices_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.contract_appendices_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.contract_appendices_id_seq OWNER TO postgres;

--
-- Name: contract_appendices_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.contract_appendices_id_seq OWNED BY public.contract_appendices.id;


--
-- Name: contracts; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.contracts (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    number character varying(50) NOT NULL,
    title character varying(200) NOT NULL,
    description text,
    company_id bigint NOT NULL,
    client_name character varying(200) NOT NULL,
    client_inn character varying(20),
    client_kpp character varying(20),
    client_email character varying(100),
    client_phone character varying(20),
    client_address text,
    start_date timestamp with time zone NOT NULL,
    end_date timestamp with time zone NOT NULL,
    signed_at timestamp with time zone,
    tariff_plan_id bigint NOT NULL,
    total_amount numeric(15,2),
    currency character varying(3) DEFAULT 'RUB'::character varying,
    status character varying(20) DEFAULT 'draft'::character varying,
    is_active boolean DEFAULT true,
    notify_before bigint DEFAULT 30,
    notes text,
    external_id character varying(100)
);


ALTER TABLE public.contracts OWNER TO postgres;

--
-- Name: contracts_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.contracts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.contracts_id_seq OWNER TO postgres;

--
-- Name: contracts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.contracts_id_seq OWNED BY public.contracts.id;


--
-- Name: equipment; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.equipment (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    type character varying(50) NOT NULL,
    model character varying(100) NOT NULL,
    brand character varying(100),
    serial_number character varying(100),
    imei character varying(20),
    phone_number character varying(20),
    mac_address character varying(20),
    qr_code character varying(100),
    status character varying(20) DEFAULT 'in_stock'::character varying,
    condition character varying(20) DEFAULT 'new'::character varying,
    object_id bigint,
    category_id bigint,
    warehouse_location character varying(100),
    purchase_price numeric(10,2),
    purchase_date timestamp with time zone,
    warranty_until timestamp with time zone,
    specifications jsonb,
    notes text,
    last_maintenance_at timestamp with time zone
);


ALTER TABLE public.equipment OWNER TO postgres;

--
-- Name: equipment_categories; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.equipment_categories (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    description text,
    code character varying(20),
    min_stock_level bigint DEFAULT 5,
    is_active boolean DEFAULT true
);


ALTER TABLE public.equipment_categories OWNER TO postgres;

--
-- Name: equipment_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.equipment_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.equipment_categories_id_seq OWNER TO postgres;

--
-- Name: equipment_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.equipment_categories_id_seq OWNED BY public.equipment_categories.id;


--
-- Name: equipment_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.equipment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.equipment_id_seq OWNER TO postgres;

--
-- Name: equipment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.equipment_id_seq OWNED BY public.equipment.id;


--
-- Name: installation_equipment; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.installation_equipment (
    equipment_id bigint NOT NULL,
    installation_id bigint NOT NULL
);


ALTER TABLE public.installation_equipment OWNER TO postgres;

--
-- Name: installations; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.installations (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    type character varying(50) NOT NULL,
    status character varying(50) DEFAULT 'planned'::character varying,
    priority character varying(20) DEFAULT 'normal'::character varying,
    description text,
    scheduled_at timestamp with time zone NOT NULL,
    estimated_duration bigint,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    object_id bigint NOT NULL,
    installer_id bigint NOT NULL,
    location_id bigint,
    client_contact character varying(100),
    address text,
    notes text,
    result text,
    created_by_user_id bigint,
    reminder_sent boolean DEFAULT false,
    reminder_sent_at timestamp with time zone,
    notification_sent boolean DEFAULT false,
    actual_duration bigint,
    travel_time bigint,
    materials_cost numeric,
    labor_cost numeric,
    quality_rating numeric,
    client_feedback text,
    issues text,
    photos text[],
    cost numeric(10,2),
    is_billable boolean DEFAULT true,
    company_id bigint
);


ALTER TABLE public.installations OWNER TO postgres;

--
-- Name: installations_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.installations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.installations_id_seq OWNER TO postgres;

--
-- Name: installations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.installations_id_seq OWNED BY public.installations.id;


--
-- Name: installer_locations; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.installer_locations (
    installer_id bigint NOT NULL,
    location_id bigint NOT NULL
);


ALTER TABLE public.installer_locations OWNER TO postgres;

--
-- Name: installers; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.installers (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    first_name character varying(50) NOT NULL,
    last_name character varying(50) NOT NULL,
    middle_name character varying(50),
    type character varying(20) NOT NULL,
    phone character varying(20) NOT NULL,
    email character varying(100),
    telegram_id character varying(50),
    specialization text[],
    skill_level character varying(20) DEFAULT 'junior'::character varying,
    experience bigint,
    location_ids integer[],
    max_daily_installations bigint DEFAULT 3,
    working_hours_start text DEFAULT '09:00'::text,
    working_hours_end text DEFAULT '18:00'::text,
    working_days integer[],
    hourly_rate numeric(8,2),
    is_active boolean DEFAULT true,
    status character varying(20) DEFAULT 'available'::character varying,
    last_worked_at timestamp with time zone,
    rating numeric DEFAULT 5,
    completed_jobs bigint DEFAULT 0,
    notes text
);


ALTER TABLE public.installers OWNER TO postgres;

--
-- Name: installers_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.installers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.installers_id_seq OWNER TO postgres;

--
-- Name: installers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.installers_id_seq OWNED BY public.installers.id;


--
-- Name: integration_errors; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.integration_errors (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    tenant_id bigint NOT NULL,
    operation character varying(50) NOT NULL,
    object_id bigint,
    external_id character varying(100),
    service character varying(50) NOT NULL,
    error_message text,
    error_code character varying(100),
    retryable boolean DEFAULT true,
    retry_count bigint DEFAULT 0,
    max_retries bigint DEFAULT 3,
    next_retry_at timestamp with time zone,
    last_retry_at timestamp with time zone,
    status character varying(50) DEFAULT 'pending'::character varying,
    resolved_at timestamp with time zone,
    resolved_by character varying(100),
    request_data text,
    response_data text,
    stack_trace text,
    user_agent character varying(255)
);


ALTER TABLE public.integration_errors OWNER TO postgres;

--
-- Name: integration_errors_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.integration_errors_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.integration_errors_id_seq OWNER TO postgres;

--
-- Name: integration_errors_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.integration_errors_id_seq OWNED BY public.integration_errors.id;


--
-- Name: integrations; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.integrations (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    company_id bigint NOT NULL,
    integration_type character varying(50) NOT NULL,
    name character varying(100) NOT NULL,
    description text,
    settings text,
    is_active boolean DEFAULT true,
    last_sync_at timestamp with time zone,
    last_error_at timestamp with time zone,
    error_message text,
    sync_count bigint DEFAULT 0,
    error_count bigint DEFAULT 0,
    success_count bigint DEFAULT 0
);


ALTER TABLE public.integrations OWNER TO postgres;

--
-- Name: integrations_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.integrations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.integrations_id_seq OWNER TO postgres;

--
-- Name: integrations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.integrations_id_seq OWNED BY public.integrations.id;


--
-- Name: local_users; Type: TABLE; Schema: public; Owner: com
--

CREATE TABLE public.local_users (
    id integer NOT NULL,
    username character varying(64) NOT NULL,
    password_hash character varying(255) NOT NULL,
    company_id character varying(36) NOT NULL,
    role character varying(32) DEFAULT 'user'::character varying NOT NULL,
    email character varying(255),
    name character varying(255),
    is_active boolean DEFAULT true,
    last_login timestamp without time zone,
    login_count integer DEFAULT 0,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone
);


ALTER TABLE public.local_users OWNER TO com;

--
-- Name: local_users_id_seq; Type: SEQUENCE; Schema: public; Owner: com
--

CREATE SEQUENCE public.local_users_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.local_users_id_seq OWNER TO com;

--
-- Name: local_users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: com
--

ALTER SEQUENCE public.local_users_id_seq OWNED BY public.local_users.id;


--
-- Name: locations; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.locations (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    city character varying(100) NOT NULL,
    region character varying(100),
    country character varying(100) DEFAULT 'Russia'::character varying,
    latitude numeric,
    longitude numeric,
    timezone character varying(50) DEFAULT 'Europe/Moscow'::character varying,
    is_active boolean DEFAULT true,
    notes text
);


ALTER TABLE public.locations OWNER TO postgres;

--
-- Name: locations_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.locations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.locations_id_seq OWNER TO postgres;

--
-- Name: locations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.locations_id_seq OWNED BY public.locations.id;


--
-- Name: notification_logs; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.notification_logs (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    type text NOT NULL,
    channel text NOT NULL,
    recipient text NOT NULL,
    subject text,
    message text NOT NULL,
    status text DEFAULT 'pending'::text,
    error_message text,
    sent_at timestamp with time zone,
    related_id bigint,
    related_type text,
    user_id bigint,
    template_id bigint,
    attempt_count bigint DEFAULT 0,
    next_retry_at timestamp with time zone,
    external_id text,
    company_id bigint
);


ALTER TABLE public.notification_logs OWNER TO postgres;

--
-- Name: notification_logs_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.notification_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.notification_logs_id_seq OWNER TO postgres;

--
-- Name: notification_logs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.notification_logs_id_seq OWNED BY public.notification_logs.id;


--
-- Name: notification_settings; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.notification_settings (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    telegram_bot_token character varying(500),
    telegram_webhook_url text,
    telegram_enabled boolean DEFAULT false,
    smtp_host text,
    smtp_port bigint DEFAULT 587,
    smtp_username text,
    smtp_password text,
    smtp_from_email text,
    smtp_from_name text,
    smtp_use_tls boolean DEFAULT true,
    email_enabled boolean DEFAULT false,
    sms_provider text,
    sms_api_key text,
    sms_api_secret text,
    sms_from_number text,
    sms_enabled boolean DEFAULT false,
    default_language text DEFAULT 'ru'::text,
    max_retry_attempts bigint DEFAULT 3,
    retry_delay_minutes bigint DEFAULT 5,
    company_id bigint
);


ALTER TABLE public.notification_settings OWNER TO postgres;

--
-- Name: notification_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.notification_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.notification_settings_id_seq OWNER TO postgres;

--
-- Name: notification_settings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.notification_settings_id_seq OWNED BY public.notification_settings.id;


--
-- Name: notification_templates; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.notification_templates (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text NOT NULL,
    type text NOT NULL,
    channel text NOT NULL,
    subject text,
    template text NOT NULL,
    description text,
    is_active boolean DEFAULT true,
    language text DEFAULT 'ru'::text,
    priority text DEFAULT 'normal'::text,
    retry_attempts bigint DEFAULT 3,
    delay_seconds bigint DEFAULT 0,
    company_id bigint
);


ALTER TABLE public.notification_templates OWNER TO postgres;

--
-- Name: notification_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.notification_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.notification_templates_id_seq OWNER TO postgres;

--
-- Name: notification_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.notification_templates_id_seq OWNED BY public.notification_templates.id;


--
-- Name: object_templates; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.object_templates (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    description text,
    category character varying(50),
    icon character varying(50),
    color character varying(7),
    config jsonb,
    default_settings jsonb,
    required_equipment text[],
    is_active boolean DEFAULT true,
    is_system boolean DEFAULT false,
    usage_count bigint DEFAULT 0
);


ALTER TABLE public.object_templates OWNER TO postgres;

--
-- Name: object_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.object_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.object_templates_id_seq OWNER TO postgres;

--
-- Name: object_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.object_templates_id_seq OWNED BY public.object_templates.id;


--
-- Name: objects; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.objects (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    type character varying(50) NOT NULL,
    description text,
    latitude numeric,
    longitude numeric,
    address text,
    imei character varying(20),
    phone_number character varying(20),
    serial_number character varying(50),
    status character varying(20) DEFAULT 'active'::character varying,
    is_active boolean DEFAULT true,
    scheduled_delete_at timestamp with time zone,
    last_activity_at timestamp with time zone,
    contract_id bigint NOT NULL,
    template_id bigint,
    location_id bigint,
    settings jsonb,
    tags text[],
    notes text,
    external_id character varying(100)
);


ALTER TABLE public.objects OWNER TO postgres;

--
-- Name: objects_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.objects_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.objects_id_seq OWNER TO postgres;

--
-- Name: objects_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.objects_id_seq OWNED BY public.objects.id;


--
-- Name: permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.permissions (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    display_name character varying(100) NOT NULL,
    description text,
    resource character varying(50) NOT NULL,
    action character varying(50) NOT NULL,
    category character varying(50),
    is_active boolean DEFAULT true
);


ALTER TABLE public.permissions OWNER TO postgres;

--
-- Name: permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.permissions_id_seq OWNER TO postgres;

--
-- Name: permissions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.permissions_id_seq OWNED BY public.permissions.id;


--
-- Name: refresh_tokens; Type: TABLE; Schema: public; Owner: com
--

CREATE TABLE public.refresh_tokens (
    id integer NOT NULL,
    user_id integer NOT NULL,
    token character varying(255) NOT NULL,
    expires_at timestamp without time zone NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    is_revoked boolean DEFAULT false
);


ALTER TABLE public.refresh_tokens OWNER TO com;

--
-- Name: refresh_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: com
--

CREATE SEQUENCE public.refresh_tokens_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.refresh_tokens_id_seq OWNER TO com;

--
-- Name: refresh_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: com
--

ALTER SEQUENCE public.refresh_tokens_id_seq OWNED BY public.refresh_tokens.id;


--
-- Name: report_executions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.report_executions (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    schedule_id bigint NOT NULL,
    report_id bigint,
    status character varying(20) DEFAULT 'pending'::character varying,
    error_msg text,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    duration bigint,
    emails_sent bigint,
    emails_failures bigint,
    delivery_log text,
    company_id bigint
);


ALTER TABLE public.report_executions OWNER TO postgres;

--
-- Name: report_executions_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.report_executions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.report_executions_id_seq OWNER TO postgres;

--
-- Name: report_executions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.report_executions_id_seq OWNED BY public.report_executions.id;


--
-- Name: report_schedules; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.report_schedules (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(200) NOT NULL,
    description text,
    type character varying(20) NOT NULL,
    template_id bigint NOT NULL,
    cron_expression character varying(100),
    time_of_day character varying(10),
    day_of_week bigint,
    day_of_month bigint,
    parameters jsonb,
    format character varying(20) NOT NULL,
    recipients jsonb,
    is_active boolean DEFAULT true,
    last_run_at timestamp with time zone,
    next_run_at timestamp with time zone,
    last_report_id bigint,
    run_count bigint DEFAULT 0,
    fail_count bigint DEFAULT 0,
    created_by_id bigint NOT NULL,
    company_id bigint
);


ALTER TABLE public.report_schedules OWNER TO postgres;

--
-- Name: report_schedules_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.report_schedules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.report_schedules_id_seq OWNER TO postgres;

--
-- Name: report_schedules_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.report_schedules_id_seq OWNED BY public.report_schedules.id;


--
-- Name: report_templates; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.report_templates (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(200) NOT NULL,
    description text,
    type character varying(50) NOT NULL,
    config jsonb,
    sql_query text,
    parameters jsonb,
    headers jsonb,
    formatting jsonb,
    is_active boolean DEFAULT true,
    is_public boolean DEFAULT false,
    created_by_id bigint NOT NULL,
    company_id bigint
);


ALTER TABLE public.report_templates OWNER TO postgres;

--
-- Name: report_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.report_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.report_templates_id_seq OWNER TO postgres;

--
-- Name: report_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.report_templates_id_seq OWNED BY public.report_templates.id;


--
-- Name: reports; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.reports (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(200) NOT NULL,
    description text,
    type character varying(50) NOT NULL,
    parameters jsonb,
    date_from timestamp with time zone,
    date_to timestamp with time zone,
    status character varying(20) DEFAULT 'pending'::character varying,
    error_msg text,
    file_path character varying(500),
    file_size bigint,
    record_count bigint,
    format character varying(20) NOT NULL,
    created_by_id bigint NOT NULL,
    company_id bigint,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    duration bigint
);


ALTER TABLE public.reports OWNER TO postgres;

--
-- Name: reports_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.reports_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.reports_id_seq OWNER TO postgres;

--
-- Name: reports_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.reports_id_seq OWNED BY public.reports.id;


--
-- Name: role_permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.role_permissions (
    role_id bigint NOT NULL,
    permission_id bigint NOT NULL
);


ALTER TABLE public.role_permissions OWNER TO postgres;

--
-- Name: roles; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.roles (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    display_name character varying(100) NOT NULL,
    description text,
    color character varying(7),
    priority bigint DEFAULT 0,
    is_active boolean DEFAULT true,
    is_system boolean DEFAULT false
);


ALTER TABLE public.roles OWNER TO postgres;

--
-- Name: roles_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.roles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.roles_id_seq OWNER TO postgres;

--
-- Name: roles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.roles_id_seq OWNED BY public.roles.id;


--
-- Name: stock_alerts; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_alerts (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    type character varying(50) NOT NULL,
    title character varying(200) NOT NULL,
    description text,
    severity character varying(20) DEFAULT 'medium'::character varying,
    equipment_id bigint,
    equipment_category_id bigint,
    status character varying(20) DEFAULT 'active'::character varying,
    read_at timestamp with time zone,
    resolved_at timestamp with time zone,
    assigned_user_id bigint,
    metadata jsonb,
    company_id bigint
);


ALTER TABLE public.stock_alerts OWNER TO postgres;

--
-- Name: stock_alerts_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.stock_alerts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.stock_alerts_id_seq OWNER TO postgres;

--
-- Name: stock_alerts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.stock_alerts_id_seq OWNED BY public.stock_alerts.id;


--
-- Name: subscriptions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.subscriptions (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    company_id bigint NOT NULL,
    billing_plan_id bigint NOT NULL,
    start_date timestamp with time zone NOT NULL,
    end_date timestamp with time zone,
    status character varying(20) DEFAULT 'active'::character varying,
    is_auto_renew boolean DEFAULT true,
    last_payment_date timestamp with time zone,
    next_payment_date timestamp with time zone,
    payment_method character varying(50)
);


ALTER TABLE public.subscriptions OWNER TO postgres;

--
-- Name: subscriptions_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.subscriptions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.subscriptions_id_seq OWNER TO postgres;

--
-- Name: subscriptions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.subscriptions_id_seq OWNED BY public.subscriptions.id;


--
-- Name: user_accesses; Type: TABLE; Schema: public; Owner: com
--

CREATE TABLE public.user_accesses (
    id integer NOT NULL,
    user_id bigint NOT NULL,
    scope character varying(100) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    perms text
);


ALTER TABLE public.user_accesses OWNER TO com;

--
-- Name: user_accesses_id_seq; Type: SEQUENCE; Schema: public; Owner: com
--

CREATE SEQUENCE public.user_accesses_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.user_accesses_id_seq OWNER TO com;

--
-- Name: user_accesses_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: com
--

ALTER SEQUENCE public.user_accesses_id_seq OWNED BY public.user_accesses.id;


--
-- Name: user_notification_preferences; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.user_notification_preferences (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    user_id bigint NOT NULL,
    telegram_enabled boolean DEFAULT true,
    email_enabled boolean DEFAULT true,
    sms_enabled boolean DEFAULT false,
    installation_reminders boolean DEFAULT true,
    installation_updates boolean DEFAULT true,
    billing_alerts boolean DEFAULT true,
    warehouse_alerts boolean DEFAULT true,
    system_notifications boolean DEFAULT true,
    quiet_hours_start text DEFAULT '22:00'::text,
    quiet_hours_end text DEFAULT '08:00'::text,
    timezone text DEFAULT 'Europe/Moscow'::text,
    company_id bigint
);


ALTER TABLE public.user_notification_preferences OWNER TO postgres;

--
-- Name: user_notification_preferences_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.user_notification_preferences_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.user_notification_preferences_id_seq OWNER TO postgres;

--
-- Name: user_notification_preferences_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.user_notification_preferences_id_seq OWNED BY public.user_notification_preferences.id;


--
-- Name: user_tabs; Type: TABLE; Schema: public; Owner: com
--

CREATE TABLE public.user_tabs (
    id integer NOT NULL,
    user_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL
);


ALTER TABLE public.user_tabs OWNER TO com;

--
-- Name: user_tabs_id_seq; Type: SEQUENCE; Schema: public; Owner: com
--

CREATE SEQUENCE public.user_tabs_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.user_tabs_id_seq OWNER TO com;

--
-- Name: user_tabs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: com
--

ALTER SEQUENCE public.user_tabs_id_seq OWNED BY public.user_tabs.id;


--
-- Name: user_templates; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.user_templates (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    description text,
    role_id bigint NOT NULL,
    settings jsonb,
    is_active boolean DEFAULT true
);


ALTER TABLE public.user_templates OWNER TO postgres;

--
-- Name: user_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.user_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.user_templates_id_seq OWNER TO postgres;

--
-- Name: user_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.user_templates_id_seq OWNED BY public.user_templates.id;


--
-- Name: user_tokens; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.user_tokens (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp without time zone,
    user_id integer NOT NULL,
    username character varying(100) NOT NULL,
    token text NOT NULL,
    expires_at timestamp without time zone,
    is_active boolean DEFAULT true,
    last_used_at timestamp without time zone,
    user_agent text,
    ip_address character varying(45)
);


ALTER TABLE public.user_tokens OWNER TO postgres;

--
-- Name: user_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.user_tokens_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.user_tokens_id_seq OWNER TO postgres;

--
-- Name: user_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.user_tokens_id_seq OWNED BY public.user_tokens.id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    username text NOT NULL,
    email text NOT NULL,
    password text NOT NULL,
    first_name text,
    last_name text,
    name character varying(200),
    phone character varying(50),
    telegram_id character varying(50),
    is_active boolean DEFAULT true,
    user_type character varying(50) DEFAULT 'user'::character varying,
    external_id character varying(100),
    external_source character varying(50),
    company_id bigint,
    role_id bigint,
    template_id bigint,
    last_login timestamp with time zone,
    login_count bigint DEFAULT 0,
    axenta_user_type character varying(50),
    axenta_user_id character varying(100),
    is_axenta_user boolean DEFAULT false
);


ALTER TABLE public.users OWNER TO postgres;

--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.users_id_seq OWNER TO postgres;

--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: warehouse_operations; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.warehouse_operations (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    type character varying(50) NOT NULL,
    description text,
    status character varying(20) DEFAULT 'completed'::character varying,
    equipment_id bigint NOT NULL,
    quantity bigint DEFAULT 1,
    from_location character varying(100),
    to_location character varying(100),
    user_id bigint,
    document_number character varying(50),
    notes text,
    installation_id bigint,
    company_id bigint
);


ALTER TABLE public.warehouse_operations OWNER TO postgres;

--
-- Name: warehouse_operations_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.warehouse_operations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.warehouse_operations_id_seq OWNER TO postgres;

--
-- Name: warehouse_operations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.warehouse_operations_id_seq OWNED BY public.warehouse_operations.id;


--
-- Name: billing_plans; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.billing_plans (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    description text,
    price numeric(10,2) NOT NULL,
    currency character varying(3) DEFAULT 'RUB'::character varying,
    billing_period character varying(20) DEFAULT 'monthly'::character varying,
    max_devices bigint DEFAULT 0,
    max_users bigint DEFAULT 0,
    max_storage bigint DEFAULT 0,
    has_analytics boolean DEFAULT false,
    has_api boolean DEFAULT false,
    has_support boolean DEFAULT false,
    has_custom_domain boolean DEFAULT false,
    is_active boolean DEFAULT true,
    is_popular boolean DEFAULT false,
    company_id bigint
);


ALTER TABLE tenant_default.billing_plans OWNER TO postgres;

--
-- Name: billing_plans_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.billing_plans_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.billing_plans_id_seq OWNER TO postgres;

--
-- Name: billing_plans_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.billing_plans_id_seq OWNED BY tenant_default.billing_plans.id;


--
-- Name: companies; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.companies (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    database_schema character varying(100) NOT NULL,
    domain character varying(100),
    axetna_login character varying(100) NOT NULL,
    axetna_password text NOT NULL,
    bitrix24_webhook_url character varying(500),
    bitrix24_client_id character varying(100),
    bitrix24_client_secret character varying(200),
    contact_email character varying(100),
    contact_phone character varying(20),
    contact_person character varying(100),
    address text,
    city character varying(100),
    country character varying(100) DEFAULT 'Russia'::character varying,
    is_active boolean DEFAULT true,
    max_users bigint DEFAULT 10,
    max_objects bigint DEFAULT 100,
    storage_quota bigint DEFAULT 1024,
    language character varying(5) DEFAULT 'ru'::character varying,
    timezone character varying(50) DEFAULT 'Europe/Moscow'::character varying,
    currency character varying(3) DEFAULT 'RUB'::character varying,
    hierarchy text,
    subscription_id text
);


ALTER TABLE tenant_default.companies OWNER TO postgres;

--
-- Name: companies_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.companies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.companies_id_seq OWNER TO postgres;

--
-- Name: companies_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.companies_id_seq OWNED BY tenant_default.companies.id;


--
-- Name: contracts; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.contracts (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    number character varying(50) NOT NULL,
    title character varying(200) NOT NULL,
    description text,
    company_id bigint NOT NULL,
    client_name character varying(200) NOT NULL,
    client_inn character varying(20),
    client_kpp character varying(20),
    client_email character varying(100),
    client_phone character varying(20),
    client_address text,
    start_date timestamp with time zone NOT NULL,
    end_date timestamp with time zone NOT NULL,
    signed_at timestamp with time zone,
    tariff_plan_id bigint NOT NULL,
    total_amount numeric(15,2),
    currency character varying(3) DEFAULT 'RUB'::character varying,
    status character varying(20) DEFAULT 'draft'::character varying,
    is_active boolean DEFAULT true,
    notify_before bigint DEFAULT 30,
    notes text,
    external_id character varying(100)
);


ALTER TABLE tenant_default.contracts OWNER TO postgres;

--
-- Name: contracts_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.contracts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.contracts_id_seq OWNER TO postgres;

--
-- Name: contracts_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.contracts_id_seq OWNED BY tenant_default.contracts.id;


--
-- Name: equipment_categories; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.equipment_categories (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    description text,
    code character varying(20),
    min_stock_level bigint DEFAULT 5,
    is_active boolean DEFAULT true
);


ALTER TABLE tenant_default.equipment_categories OWNER TO postgres;

--
-- Name: equipment_categories_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.equipment_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.equipment_categories_id_seq OWNER TO postgres;

--
-- Name: equipment_categories_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.equipment_categories_id_seq OWNED BY tenant_default.equipment_categories.id;


--
-- Name: installers; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.installers (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    first_name character varying(50) NOT NULL,
    last_name character varying(50) NOT NULL,
    middle_name character varying(50),
    type character varying(20) NOT NULL,
    phone character varying(20) NOT NULL,
    email character varying(100),
    telegram_id character varying(50),
    specialization text[],
    skill_level character varying(20) DEFAULT 'junior'::character varying,
    experience bigint,
    location_ids integer[],
    max_daily_installations bigint DEFAULT 3,
    working_hours_start text DEFAULT '09:00'::text,
    working_hours_end text DEFAULT '18:00'::text,
    working_days integer[],
    hourly_rate numeric(8,2),
    is_active boolean DEFAULT true,
    status character varying(20) DEFAULT 'available'::character varying,
    last_worked_at timestamp with time zone,
    rating numeric DEFAULT 5,
    completed_jobs bigint DEFAULT 0,
    notes text
);


ALTER TABLE tenant_default.installers OWNER TO postgres;

--
-- Name: installers_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.installers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.installers_id_seq OWNER TO postgres;

--
-- Name: installers_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.installers_id_seq OWNED BY tenant_default.installers.id;


--
-- Name: locations; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.locations (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    city character varying(100) NOT NULL,
    region character varying(100),
    country character varying(100) DEFAULT 'Russia'::character varying,
    latitude numeric,
    longitude numeric,
    timezone character varying(50) DEFAULT 'Europe/Moscow'::character varying,
    is_active boolean DEFAULT true,
    notes text
);


ALTER TABLE tenant_default.locations OWNER TO postgres;

--
-- Name: locations_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.locations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.locations_id_seq OWNER TO postgres;

--
-- Name: locations_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.locations_id_seq OWNED BY tenant_default.locations.id;


--
-- Name: monitoring_notification_templates; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.monitoring_notification_templates (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    description text,
    type character varying(50) NOT NULL,
    event_type character varying(50) NOT NULL,
    email_subject character varying(200),
    email_body text,
    sms_message character varying(160),
    telegram_message text,
    webhook_payload text,
    priority character varying(20) DEFAULT 'normal'::character varying,
    retry_count bigint DEFAULT 3,
    retry_interval bigint DEFAULT 300,
    max_per_hour bigint DEFAULT 0,
    max_per_day bigint DEFAULT 0,
    active_from timestamp with time zone,
    active_until timestamp with time zone,
    week_days bigint DEFAULT 127,
    time_from character varying(5),
    time_until character varying(5),
    is_active boolean DEFAULT true,
    usage_count bigint DEFAULT 0,
    variables jsonb
);


ALTER TABLE tenant_default.monitoring_notification_templates OWNER TO postgres;

--
-- Name: monitoring_notification_templates_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.monitoring_notification_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.monitoring_notification_templates_id_seq OWNER TO postgres;

--
-- Name: monitoring_notification_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.monitoring_notification_templates_id_seq OWNED BY tenant_default.monitoring_notification_templates.id;


--
-- Name: monitoring_templates; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.monitoring_templates (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    description text,
    check_interval bigint DEFAULT 300,
    alert_threshold bigint DEFAULT 600,
    geo_fence_enabled boolean DEFAULT false,
    speed_limit bigint DEFAULT 0,
    notify_on_offline boolean DEFAULT true,
    notify_on_move boolean DEFAULT false,
    notify_on_speed boolean DEFAULT false,
    notify_on_geo_fence boolean DEFAULT false,
    email_enabled boolean DEFAULT true,
    sms_enabled boolean DEFAULT false,
    telegram_enabled boolean DEFAULT false,
    webhook_enabled boolean DEFAULT false,
    settings jsonb,
    is_active boolean DEFAULT true,
    usage_count bigint DEFAULT 0
);


ALTER TABLE tenant_default.monitoring_templates OWNER TO postgres;

--
-- Name: monitoring_templates_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.monitoring_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.monitoring_templates_id_seq OWNER TO postgres;

--
-- Name: monitoring_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.monitoring_templates_id_seq OWNED BY tenant_default.monitoring_templates.id;


--
-- Name: object_templates; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.object_templates (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    description text,
    category character varying(50),
    icon character varying(50),
    color character varying(7),
    config jsonb,
    default_settings jsonb,
    required_equipment text[],
    is_active boolean DEFAULT true,
    is_system boolean DEFAULT false,
    usage_count bigint DEFAULT 0
);


ALTER TABLE tenant_default.object_templates OWNER TO postgres;

--
-- Name: object_templates_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.object_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.object_templates_id_seq OWNER TO postgres;

--
-- Name: object_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.object_templates_id_seq OWNED BY tenant_default.object_templates.id;


--
-- Name: objects; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.objects (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    type character varying(50) NOT NULL,
    description text,
    latitude numeric,
    longitude numeric,
    address text,
    imei character varying(20),
    phone_number character varying(20),
    serial_number character varying(50),
    status character varying(20) DEFAULT 'active'::character varying,
    is_active boolean DEFAULT true,
    scheduled_delete_at timestamp with time zone,
    last_activity_at timestamp with time zone,
    company_id bigint NOT NULL,
    contract_id bigint NOT NULL,
    template_id bigint,
    location_id bigint,
    settings jsonb,
    tags text[],
    notes text,
    external_id character varying(100)
);


ALTER TABLE tenant_default.objects OWNER TO postgres;

--
-- Name: objects_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.objects_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.objects_id_seq OWNER TO postgres;

--
-- Name: objects_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.objects_id_seq OWNED BY tenant_default.objects.id;


--
-- Name: permissions; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.permissions (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    display_name character varying(100) NOT NULL,
    description text,
    resource character varying(50) NOT NULL,
    action character varying(50) NOT NULL,
    category character varying(50),
    is_active boolean DEFAULT true
);


ALTER TABLE tenant_default.permissions OWNER TO postgres;

--
-- Name: permissions_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.permissions_id_seq OWNER TO postgres;

--
-- Name: permissions_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.permissions_id_seq OWNED BY tenant_default.permissions.id;


--
-- Name: role_permissions; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.role_permissions (
    role_id integer NOT NULL,
    permission_id integer NOT NULL
);


ALTER TABLE tenant_default.role_permissions OWNER TO postgres;

--
-- Name: roles; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.roles (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    display_name character varying(100) NOT NULL,
    description text,
    color character varying(7),
    priority bigint DEFAULT 0,
    is_active boolean DEFAULT true,
    is_system boolean DEFAULT false
);


ALTER TABLE tenant_default.roles OWNER TO postgres;

--
-- Name: roles_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.roles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.roles_id_seq OWNER TO postgres;

--
-- Name: roles_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.roles_id_seq OWNED BY tenant_default.roles.id;


--
-- Name: tariff_plans; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.tariff_plans (
    id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name character varying(100) NOT NULL,
    description text,
    price numeric(10,2) NOT NULL,
    currency character varying(3) DEFAULT 'RUB'::character varying,
    billing_period character varying(20) DEFAULT 'monthly'::character varying,
    max_devices bigint DEFAULT 0,
    max_users bigint DEFAULT 0,
    max_storage bigint DEFAULT 0,
    has_analytics boolean DEFAULT false,
    has_api boolean DEFAULT false,
    has_support boolean DEFAULT false,
    has_custom_domain boolean DEFAULT false,
    is_active boolean DEFAULT true,
    is_popular boolean DEFAULT false,
    company_id bigint,
    setup_fee numeric(10,2) DEFAULT '0'::numeric,
    minimum_period bigint DEFAULT 1,
    discount_percent numeric(5,2) DEFAULT '0'::numeric,
    is_promotional boolean DEFAULT false,
    promotional_until timestamp with time zone,
    price_per_object numeric(10,2),
    free_objects_count bigint DEFAULT 0,
    inactive_price_ratio numeric(3,2) DEFAULT 0.5
);


ALTER TABLE tenant_default.tariff_plans OWNER TO postgres;

--
-- Name: tariff_plans_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.tariff_plans_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.tariff_plans_id_seq OWNER TO postgres;

--
-- Name: tariff_plans_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.tariff_plans_id_seq OWNED BY tenant_default.tariff_plans.id;


--
-- Name: user_templates; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.user_templates (
    id integer NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    name character varying(255) NOT NULL,
    description text,
    role_id integer,
    settings jsonb,
    is_active boolean DEFAULT true
);


ALTER TABLE tenant_default.user_templates OWNER TO postgres;

--
-- Name: user_templates_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.user_templates_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.user_templates_id_seq OWNER TO postgres;

--
-- Name: user_templates_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.user_templates_id_seq OWNED BY tenant_default.user_templates.id;


--
-- Name: users; Type: TABLE; Schema: tenant_default; Owner: postgres
--

CREATE TABLE tenant_default.users (
    id integer NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp with time zone,
    username character varying(255) NOT NULL,
    email character varying(255) NOT NULL,
    password character varying(255) NOT NULL,
    first_name character varying(255),
    last_name character varying(255),
    name character varying(200),
    phone character varying(50),
    telegram_id character varying(50),
    is_active boolean DEFAULT true,
    user_type character varying(50) DEFAULT 'user'::character varying,
    external_id character varying(100),
    external_source character varying(50),
    axenta_user_type character varying(50),
    axenta_user_id character varying(100),
    is_axenta_user boolean DEFAULT false,
    company_id integer,
    role_id integer,
    template_id integer,
    last_login timestamp with time zone,
    login_count integer DEFAULT 0
);


ALTER TABLE tenant_default.users OWNER TO postgres;

--
-- Name: users_id_seq; Type: SEQUENCE; Schema: tenant_default; Owner: postgres
--

CREATE SEQUENCE tenant_default.users_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_default.users_id_seq OWNER TO postgres;

--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_default; Owner: postgres
--

ALTER SEQUENCE tenant_default.users_id_seq OWNED BY tenant_default.users.id;


--
-- Name: equipment; Type: TABLE; Schema: tenant_newacrm; Owner: postgres
--

CREATE TABLE tenant_newacrm.equipment (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone DEFAULT now(),
    deleted_at timestamp without time zone,
    type character varying(100) NOT NULL,
    model character varying(100),
    serial_number character varying(100),
    status character varying(50) DEFAULT 'in_stock'::character varying,
    company_id integer
);


ALTER TABLE tenant_newacrm.equipment OWNER TO postgres;

--
-- Name: equipment_id_seq; Type: SEQUENCE; Schema: tenant_newacrm; Owner: postgres
--

CREATE SEQUENCE tenant_newacrm.equipment_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_newacrm.equipment_id_seq OWNER TO postgres;

--
-- Name: equipment_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_newacrm; Owner: postgres
--

ALTER SEQUENCE tenant_newacrm.equipment_id_seq OWNED BY tenant_newacrm.equipment.id;


--
-- Name: installations; Type: TABLE; Schema: tenant_newacrm; Owner: postgres
--

CREATE TABLE tenant_newacrm.installations (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone DEFAULT now(),
    deleted_at timestamp without time zone,
    type character varying(50) NOT NULL,
    status character varying(50) DEFAULT 'planned'::character varying,
    priority character varying(20) DEFAULT 'normal'::character varying,
    description text,
    scheduled_at timestamp without time zone NOT NULL,
    estimated_duration integer,
    object_id integer NOT NULL,
    installer_id integer NOT NULL,
    location_id integer,
    client_contact character varying(100),
    address text,
    notes text,
    company_id integer
);


ALTER TABLE tenant_newacrm.installations OWNER TO postgres;

--
-- Name: installations_id_seq; Type: SEQUENCE; Schema: tenant_newacrm; Owner: postgres
--

CREATE SEQUENCE tenant_newacrm.installations_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_newacrm.installations_id_seq OWNER TO postgres;

--
-- Name: installations_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_newacrm; Owner: postgres
--

ALTER SEQUENCE tenant_newacrm.installations_id_seq OWNED BY tenant_newacrm.installations.id;


--
-- Name: installers; Type: TABLE; Schema: tenant_newacrm; Owner: postgres
--

CREATE TABLE tenant_newacrm.installers (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone DEFAULT now(),
    deleted_at timestamp without time zone,
    first_name character varying(100) NOT NULL,
    last_name character varying(100) NOT NULL,
    phone character varying(20),
    email character varying(100),
    type character varying(20) DEFAULT 'staff'::character varying,
    is_active boolean DEFAULT true,
    company_id integer
);


ALTER TABLE tenant_newacrm.installers OWNER TO postgres;

--
-- Name: installers_id_seq; Type: SEQUENCE; Schema: tenant_newacrm; Owner: postgres
--

CREATE SEQUENCE tenant_newacrm.installers_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_newacrm.installers_id_seq OWNER TO postgres;

--
-- Name: installers_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_newacrm; Owner: postgres
--

ALTER SEQUENCE tenant_newacrm.installers_id_seq OWNED BY tenant_newacrm.installers.id;


--
-- Name: locations; Type: TABLE; Schema: tenant_newacrm; Owner: postgres
--

CREATE TABLE tenant_newacrm.locations (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone DEFAULT now(),
    deleted_at timestamp without time zone,
    city character varying(100) NOT NULL,
    region character varying(100),
    country character varying(100) DEFAULT 'Russia'::character varying,
    is_active boolean DEFAULT true,
    company_id integer
);


ALTER TABLE tenant_newacrm.locations OWNER TO postgres;

--
-- Name: locations_id_seq; Type: SEQUENCE; Schema: tenant_newacrm; Owner: postgres
--

CREATE SEQUENCE tenant_newacrm.locations_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE tenant_newacrm.locations_id_seq OWNER TO postgres;

--
-- Name: locations_id_seq; Type: SEQUENCE OWNED BY; Schema: tenant_newacrm; Owner: postgres
--

ALTER SEQUENCE tenant_newacrm.locations_id_seq OWNED BY tenant_newacrm.locations.id;


--
-- Name: billing_plans id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.billing_plans ALTER COLUMN id SET DEFAULT nextval('public.billing_plans_id_seq'::regclass);


--
-- Name: companies id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.companies ALTER COLUMN id SET DEFAULT nextval('public.companies_id_seq'::regclass);


--
-- Name: contract_appendices id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.contract_appendices ALTER COLUMN id SET DEFAULT nextval('public.contract_appendices_id_seq'::regclass);


--
-- Name: contracts id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.contracts ALTER COLUMN id SET DEFAULT nextval('public.contracts_id_seq'::regclass);


--
-- Name: equipment id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.equipment ALTER COLUMN id SET DEFAULT nextval('public.equipment_id_seq'::regclass);


--
-- Name: equipment_categories id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.equipment_categories ALTER COLUMN id SET DEFAULT nextval('public.equipment_categories_id_seq'::regclass);


--
-- Name: installations id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installations ALTER COLUMN id SET DEFAULT nextval('public.installations_id_seq'::regclass);


--
-- Name: installers id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installers ALTER COLUMN id SET DEFAULT nextval('public.installers_id_seq'::regclass);


--
-- Name: integration_errors id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.integration_errors ALTER COLUMN id SET DEFAULT nextval('public.integration_errors_id_seq'::regclass);


--
-- Name: integrations id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.integrations ALTER COLUMN id SET DEFAULT nextval('public.integrations_id_seq'::regclass);


--
-- Name: local_users id; Type: DEFAULT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.local_users ALTER COLUMN id SET DEFAULT nextval('public.local_users_id_seq'::regclass);


--
-- Name: locations id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.locations ALTER COLUMN id SET DEFAULT nextval('public.locations_id_seq'::regclass);


--
-- Name: notification_logs id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.notification_logs ALTER COLUMN id SET DEFAULT nextval('public.notification_logs_id_seq'::regclass);


--
-- Name: notification_settings id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.notification_settings ALTER COLUMN id SET DEFAULT nextval('public.notification_settings_id_seq'::regclass);


--
-- Name: notification_templates id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.notification_templates ALTER COLUMN id SET DEFAULT nextval('public.notification_templates_id_seq'::regclass);


--
-- Name: object_templates id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.object_templates ALTER COLUMN id SET DEFAULT nextval('public.object_templates_id_seq'::regclass);


--
-- Name: objects id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.objects ALTER COLUMN id SET DEFAULT nextval('public.objects_id_seq'::regclass);


--
-- Name: permissions id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions ALTER COLUMN id SET DEFAULT nextval('public.permissions_id_seq'::regclass);


--
-- Name: refresh_tokens id; Type: DEFAULT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.refresh_tokens ALTER COLUMN id SET DEFAULT nextval('public.refresh_tokens_id_seq'::regclass);


--
-- Name: report_executions id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_executions ALTER COLUMN id SET DEFAULT nextval('public.report_executions_id_seq'::regclass);


--
-- Name: report_schedules id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_schedules ALTER COLUMN id SET DEFAULT nextval('public.report_schedules_id_seq'::regclass);


--
-- Name: report_templates id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_templates ALTER COLUMN id SET DEFAULT nextval('public.report_templates_id_seq'::regclass);


--
-- Name: reports id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.reports ALTER COLUMN id SET DEFAULT nextval('public.reports_id_seq'::regclass);


--
-- Name: roles id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles ALTER COLUMN id SET DEFAULT nextval('public.roles_id_seq'::regclass);


--
-- Name: stock_alerts id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_alerts ALTER COLUMN id SET DEFAULT nextval('public.stock_alerts_id_seq'::regclass);


--
-- Name: subscriptions id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.subscriptions ALTER COLUMN id SET DEFAULT nextval('public.subscriptions_id_seq'::regclass);


--
-- Name: user_accesses id; Type: DEFAULT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.user_accesses ALTER COLUMN id SET DEFAULT nextval('public.user_accesses_id_seq'::regclass);


--
-- Name: user_notification_preferences id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_notification_preferences ALTER COLUMN id SET DEFAULT nextval('public.user_notification_preferences_id_seq'::regclass);


--
-- Name: user_tabs id; Type: DEFAULT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.user_tabs ALTER COLUMN id SET DEFAULT nextval('public.user_tabs_id_seq'::regclass);


--
-- Name: user_templates id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_templates ALTER COLUMN id SET DEFAULT nextval('public.user_templates_id_seq'::regclass);


--
-- Name: user_tokens id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_tokens ALTER COLUMN id SET DEFAULT nextval('public.user_tokens_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: warehouse_operations id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.warehouse_operations ALTER COLUMN id SET DEFAULT nextval('public.warehouse_operations_id_seq'::regclass);


--
-- Name: billing_plans id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.billing_plans ALTER COLUMN id SET DEFAULT nextval('tenant_default.billing_plans_id_seq'::regclass);


--
-- Name: companies id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.companies ALTER COLUMN id SET DEFAULT nextval('tenant_default.companies_id_seq'::regclass);


--
-- Name: contracts id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.contracts ALTER COLUMN id SET DEFAULT nextval('tenant_default.contracts_id_seq'::regclass);


--
-- Name: equipment_categories id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.equipment_categories ALTER COLUMN id SET DEFAULT nextval('tenant_default.equipment_categories_id_seq'::regclass);


--
-- Name: installers id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.installers ALTER COLUMN id SET DEFAULT nextval('tenant_default.installers_id_seq'::regclass);


--
-- Name: locations id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.locations ALTER COLUMN id SET DEFAULT nextval('tenant_default.locations_id_seq'::regclass);


--
-- Name: monitoring_notification_templates id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.monitoring_notification_templates ALTER COLUMN id SET DEFAULT nextval('tenant_default.monitoring_notification_templates_id_seq'::regclass);


--
-- Name: monitoring_templates id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.monitoring_templates ALTER COLUMN id SET DEFAULT nextval('tenant_default.monitoring_templates_id_seq'::regclass);


--
-- Name: object_templates id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.object_templates ALTER COLUMN id SET DEFAULT nextval('tenant_default.object_templates_id_seq'::regclass);


--
-- Name: objects id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.objects ALTER COLUMN id SET DEFAULT nextval('tenant_default.objects_id_seq'::regclass);


--
-- Name: permissions id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.permissions ALTER COLUMN id SET DEFAULT nextval('tenant_default.permissions_id_seq'::regclass);


--
-- Name: roles id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.roles ALTER COLUMN id SET DEFAULT nextval('tenant_default.roles_id_seq'::regclass);


--
-- Name: tariff_plans id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.tariff_plans ALTER COLUMN id SET DEFAULT nextval('tenant_default.tariff_plans_id_seq'::regclass);


--
-- Name: user_templates id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.user_templates ALTER COLUMN id SET DEFAULT nextval('tenant_default.user_templates_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.users ALTER COLUMN id SET DEFAULT nextval('tenant_default.users_id_seq'::regclass);


--
-- Name: equipment id; Type: DEFAULT; Schema: tenant_newacrm; Owner: postgres
--

ALTER TABLE ONLY tenant_newacrm.equipment ALTER COLUMN id SET DEFAULT nextval('tenant_newacrm.equipment_id_seq'::regclass);


--
-- Name: installations id; Type: DEFAULT; Schema: tenant_newacrm; Owner: postgres
--

ALTER TABLE ONLY tenant_newacrm.installations ALTER COLUMN id SET DEFAULT nextval('tenant_newacrm.installations_id_seq'::regclass);


--
-- Name: installers id; Type: DEFAULT; Schema: tenant_newacrm; Owner: postgres
--

ALTER TABLE ONLY tenant_newacrm.installers ALTER COLUMN id SET DEFAULT nextval('tenant_newacrm.installers_id_seq'::regclass);


--
-- Name: locations id; Type: DEFAULT; Schema: tenant_newacrm; Owner: postgres
--

ALTER TABLE ONLY tenant_newacrm.locations ALTER COLUMN id SET DEFAULT nextval('tenant_newacrm.locations_id_seq'::regclass);


--
-- Data for Name: billing_plans; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.billing_plans (id, created_at, updated_at, deleted_at, name, description, price, currency, billing_period, max_devices, max_users, max_storage, has_analytics, has_api, has_support, has_custom_domain, is_active, is_popular, company_id) FROM stdin;
\.


--
-- Data for Name: companies; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.companies (id, created_at, updated_at, deleted_at, name, database_schema, domain, axetna_login, axetna_password, bitrix24_webhook_url, bitrix24_client_id, bitrix24_client_secret, contact_email, contact_phone, contact_person, address, city, country, is_active, max_users, max_objects, storage_quota, language, timezone, currency, subscription_id) FROM stdin;
6	2025-10-12 17:29:45.53992+05	2025-10-12 17:29:45.53992+05	\N	Компания по умолчанию	tenant_default	\N	default	encrypted_password	\N	\N	\N	admin@example.com	\N	\N	\N	\N	Russia	t	10	100	1024	ru	Europe/Moscow	RUB	\N
\.


--
-- Data for Name: contract_appendices; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.contract_appendices (id, created_at, updated_at, deleted_at, contract_id, number, title, description, start_date, end_date, signed_at, amount, currency, status, is_active, notes, external_id) FROM stdin;
\.


--
-- Data for Name: contracts; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.contracts (id, created_at, updated_at, deleted_at, number, title, description, company_id, client_name, client_inn, client_kpp, client_email, client_phone, client_address, start_date, end_date, signed_at, tariff_plan_id, total_amount, currency, status, is_active, notify_before, notes, external_id) FROM stdin;
\.


--
-- Data for Name: equipment; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.equipment (id, created_at, updated_at, deleted_at, type, model, brand, serial_number, imei, phone_number, mac_address, qr_code, status, condition, object_id, category_id, warehouse_location, purchase_price, purchase_date, warranty_until, specifications, notes, last_maintenance_at) FROM stdin;
\.


--
-- Data for Name: equipment_categories; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.equipment_categories (id, created_at, updated_at, deleted_at, name, description, code, min_stock_level, is_active) FROM stdin;
\.


--
-- Data for Name: installation_equipment; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.installation_equipment (equipment_id, installation_id) FROM stdin;
\.


--
-- Data for Name: installations; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.installations (id, created_at, updated_at, deleted_at, type, status, priority, description, scheduled_at, estimated_duration, started_at, completed_at, object_id, installer_id, location_id, client_contact, address, notes, result, created_by_user_id, reminder_sent, reminder_sent_at, notification_sent, actual_duration, travel_time, materials_cost, labor_cost, quality_rating, client_feedback, issues, photos, cost, is_billable, company_id) FROM stdin;
\.


--
-- Data for Name: installer_locations; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.installer_locations (installer_id, location_id) FROM stdin;
\.


--
-- Data for Name: installers; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.installers (id, created_at, updated_at, deleted_at, first_name, last_name, middle_name, type, phone, email, telegram_id, specialization, skill_level, experience, location_ids, max_daily_installations, working_hours_start, working_hours_end, working_days, hourly_rate, is_active, status, last_worked_at, rating, completed_jobs, notes) FROM stdin;
\.


--
-- Data for Name: integration_errors; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.integration_errors (id, created_at, updated_at, deleted_at, tenant_id, operation, object_id, external_id, service, error_message, error_code, retryable, retry_count, max_retries, next_retry_at, last_retry_at, status, resolved_at, resolved_by, request_data, response_data, stack_trace, user_agent) FROM stdin;
\.


--
-- Data for Name: integrations; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.integrations (id, created_at, updated_at, deleted_at, company_id, integration_type, name, description, settings, is_active, last_sync_at, last_error_at, error_message, sync_count, error_count, success_count) FROM stdin;
\.


--
-- Data for Name: local_users; Type: TABLE DATA; Schema: public; Owner: com
--

COPY public.local_users (id, username, password_hash, company_id, role, email, name, is_active, last_login, login_count, created_at, updated_at, deleted_at) FROM stdin;
1	glomos	$2a$10$tKMuzTQF8CtkOh/Y.F0TLOsx2CT1sY4KKPDtaGfc8lSMRVOibTnJi	partner-company-id	admin	chudin@glomos.ru	Чудин Андрей Геннадьевич	t	2025-10-12 18:46:43.298907	12	2025-10-12 07:36:18.930142	2025-10-12 18:46:43.299069	\N
\.


--
-- Data for Name: locations; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.locations (id, created_at, updated_at, deleted_at, city, region, country, latitude, longitude, timezone, is_active, notes) FROM stdin;
\.


--
-- Data for Name: notification_logs; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.notification_logs (id, created_at, updated_at, deleted_at, type, channel, recipient, subject, message, status, error_message, sent_at, related_id, related_type, user_id, template_id, attempt_count, next_retry_at, external_id, company_id) FROM stdin;
\.


--
-- Data for Name: notification_settings; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.notification_settings (id, created_at, updated_at, deleted_at, telegram_bot_token, telegram_webhook_url, telegram_enabled, smtp_host, smtp_port, smtp_username, smtp_password, smtp_from_email, smtp_from_name, smtp_use_tls, email_enabled, sms_provider, sms_api_key, sms_api_secret, sms_from_number, sms_enabled, default_language, max_retry_attempts, retry_delay_minutes, company_id) FROM stdin;
\.


--
-- Data for Name: notification_templates; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.notification_templates (id, created_at, updated_at, deleted_at, name, type, channel, subject, template, description, is_active, language, priority, retry_attempts, delay_seconds, company_id) FROM stdin;
\.


--
-- Data for Name: object_templates; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.object_templates (id, created_at, updated_at, deleted_at, name, description, category, icon, color, config, default_settings, required_equipment, is_active, is_system, usage_count) FROM stdin;
\.


--
-- Data for Name: objects; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.objects (id, created_at, updated_at, deleted_at, name, type, description, latitude, longitude, address, imei, phone_number, serial_number, status, is_active, scheduled_delete_at, last_activity_at, contract_id, template_id, location_id, settings, tags, notes, external_id) FROM stdin;
\.


--
-- Data for Name: permissions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.permissions (id, created_at, updated_at, deleted_at, name, display_name, description, resource, action, category, is_active) FROM stdin;
\.


--
-- Data for Name: refresh_tokens; Type: TABLE DATA; Schema: public; Owner: com
--

COPY public.refresh_tokens (id, user_id, token, expires_at, created_at, is_revoked) FROM stdin;
1	1	d58a064945d0b0f9baf2eff8ca43fa76b7eca334058357bd452b996960f3e73b	2025-10-19 07:36:27.767787	2025-10-12 07:36:27.768275	f
2	1	a15884bce72225646917160fce89dfcbac8bfb325f896163a6263b1858fc5ed4	2025-10-19 07:38:19.154568	2025-10-12 07:38:19.155164	f
3	1	ca5372be20f31fc8d9c7ebb64adaf44a1867508c19bbd2bd95c83f1e79300bf9	2025-10-19 07:39:40.401711	2025-10-12 07:39:40.402117	f
4	1	c792e471e13b2ec99c9c4ce0cfe3d64229c2f63109fe4b7e534a6551519113d7	2025-10-19 07:39:49.751574	2025-10-12 07:39:49.751681	f
5	1	ae591697004b8a64b0dab8dbd4ce06213b0b062325374f9315581ae886730900	2025-10-19 07:41:38.523465	2025-10-12 07:41:38.523734	f
6	1	087e0b5e2825faeb0d04f3ce44bd354c966781f3ac78781aca8932a64cb40cb7	2025-10-19 07:42:45.199585	2025-10-12 07:42:45.199849	f
7	1	a49287f1f7c3fff038789cde7e51792f229878a62bc0c431fa11bcf9f6b043b1	2025-10-19 07:43:22.292149	2025-10-12 07:43:22.292368	f
8	1	0cfa8a878f133bb946d989c606ad7d5f4ce95e33ab59a93658654b4119733954	2025-10-19 07:48:05.588323	2025-10-12 07:48:05.588576	f
9	1	3929076a0a5400b3646b82dd44657900f6028ceffd6c135aee79f2f01573dcb2	2025-10-19 18:45:44.504085	2025-10-12 18:45:44.504746	f
10	1	f9379df6d6be574ad05b990df74b9b01d82f0aeb25917d383b9807b25af54109	2025-10-19 18:46:04.490389	2025-10-12 18:46:04.490719	f
11	1	c3c74048a55cdeb7d0e1a5ab1d8efdb605962238f6444663a0832359bd7d977d	2025-10-19 18:46:27.864454	2025-10-12 18:46:27.865294	f
12	1	772cfe54f1075cd0695b4e6635ec16a1ffcb437ec6940f6c8464d8af7fb3a27c	2025-10-19 18:46:43.29609	2025-10-12 18:46:43.297497	f
\.


--
-- Data for Name: report_executions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.report_executions (id, created_at, updated_at, deleted_at, schedule_id, report_id, status, error_msg, started_at, completed_at, duration, emails_sent, emails_failures, delivery_log, company_id) FROM stdin;
\.


--
-- Data for Name: report_schedules; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.report_schedules (id, created_at, updated_at, deleted_at, name, description, type, template_id, cron_expression, time_of_day, day_of_week, day_of_month, parameters, format, recipients, is_active, last_run_at, next_run_at, last_report_id, run_count, fail_count, created_by_id, company_id) FROM stdin;
\.


--
-- Data for Name: report_templates; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.report_templates (id, created_at, updated_at, deleted_at, name, description, type, config, sql_query, parameters, headers, formatting, is_active, is_public, created_by_id, company_id) FROM stdin;
\.


--
-- Data for Name: reports; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.reports (id, created_at, updated_at, deleted_at, name, description, type, parameters, date_from, date_to, status, error_msg, file_path, file_size, record_count, format, created_by_id, company_id, started_at, completed_at, duration) FROM stdin;
\.


--
-- Data for Name: role_permissions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.role_permissions (role_id, permission_id) FROM stdin;
\.


--
-- Data for Name: roles; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.roles (id, created_at, updated_at, deleted_at, name, display_name, description, color, priority, is_active, is_system) FROM stdin;
1	2025-10-12 16:15:32.221774+05	2025-10-12 16:15:32.221774+05	\N	partner	Партнер	Роль партнера из Axenta	#2196F3	100	t	t
2	2025-10-12 16:15:32.228268+05	2025-10-12 16:15:32.228268+05	\N	client	Клиент	Роль клиента из Axenta	#4CAF50	50	t	t
3	2025-10-12 16:15:32.229056+05	2025-10-12 16:15:32.229056+05	\N	user	Пользователь	Локальный пользователь системы	#FF9800	25	t	t
\.


--
-- Data for Name: stock_alerts; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.stock_alerts (id, created_at, updated_at, deleted_at, type, title, description, severity, equipment_id, equipment_category_id, status, read_at, resolved_at, assigned_user_id, metadata, company_id) FROM stdin;
\.


--
-- Data for Name: subscriptions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.subscriptions (id, created_at, updated_at, deleted_at, company_id, billing_plan_id, start_date, end_date, status, is_auto_renew, last_payment_date, next_payment_date, payment_method) FROM stdin;
\.


--
-- Data for Name: user_accesses; Type: TABLE DATA; Schema: public; Owner: com
--

COPY public.user_accesses (id, user_id, scope, created_at, updated_at, deleted_at, perms) FROM stdin;
1	16	objects	2025-10-16 00:05:10.65712+05	2025-10-16 00:05:10.65712+05	\N	["view","edit"]
10	20	common	2025-10-16 00:27:38.209175+05	2025-10-16 00:27:38.209175+05	\N	["view","edit"]
11	21	common	2025-10-16 00:28:00.812938+05	2025-10-16 00:28:00.812938+05	\N	["view"]
12	22	common	2025-10-16 00:28:26.953923+05	2025-10-16 00:28:26.953923+05	\N	["view"]
13	25	common	2025-10-16 10:46:34.91758+05	2025-10-16 10:46:34.91758+05	\N	["view"]
14	26	objects	2025-10-16 10:48:58.281394+05	2025-10-16 10:48:58.281394+05	\N	{"perms":["view"]}
15	26	users	2025-10-16 10:48:58.281666+05	2025-10-16 10:48:58.281666+05	\N	{"perms":["view"]}
16	26	reports	2025-10-16 10:48:58.281846+05	2025-10-16 10:48:58.281846+05	\N	{"perms":["view"]}
17	26	monitoring	2025-10-16 10:48:58.282023+05	2025-10-16 10:48:58.282023+05	\N	{"perms":["view"]}
18	27	reports	2025-10-16 10:59:30.817752+05	2025-10-16 10:59:30.817752+05	\N	{"perms":["view"]}
19	27	monitoring	2025-10-16 10:59:30.820411+05	2025-10-16 10:59:30.820411+05	\N	{"perms":["view"]}
20	27	objects	2025-10-16 10:59:30.820605+05	2025-10-16 10:59:30.820605+05	\N	{"perms":["view"]}
21	27	users	2025-10-16 10:59:30.820783+05	2025-10-16 10:59:30.820783+05	\N	{"perms":["view"]}
22	28	objects	2025-10-16 11:00:42.389859+05	2025-10-16 11:00:42.389859+05	\N	{"perms":["view"]}
23	28	users	2025-10-16 11:00:42.390124+05	2025-10-16 11:00:42.390124+05	\N	{"perms":["view"]}
24	28	reports	2025-10-16 11:00:42.390393+05	2025-10-16 11:00:42.390393+05	\N	{"perms":["view"]}
25	28	monitoring	2025-10-16 11:00:42.390616+05	2025-10-16 11:00:42.390616+05	\N	{"perms":["view"]}
26	29	common	2025-10-16 12:16:29.83836+05	2025-10-16 12:16:29.83836+05	\N	["view"]
\.


--
-- Data for Name: user_notification_preferences; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.user_notification_preferences (id, created_at, updated_at, deleted_at, user_id, telegram_enabled, email_enabled, sms_enabled, installation_reminders, installation_updates, billing_alerts, warehouse_alerts, system_notifications, quiet_hours_start, quiet_hours_end, timezone, company_id) FROM stdin;
\.


--
-- Data for Name: user_tabs; Type: TABLE DATA; Schema: public; Owner: com
--

COPY public.user_tabs (id, user_id, created_at, updated_at, deleted_at, name) FROM stdin;
2	16	2025-10-16 00:05:10.651023+05	2025-10-16 00:05:10.651023+05	\N	dashboard
3	16	2025-10-16 00:05:10.655066+05	2025-10-16 00:05:10.655066+05	\N	users
8	20	2025-10-16 00:27:38.205728+05	2025-10-16 00:27:38.205728+05	\N	monitoring
9	20	2025-10-16 00:27:38.208781+05	2025-10-16 00:27:38.208781+05	\N	reports
10	21	2025-10-16 00:28:00.81126+05	2025-10-16 00:28:00.81126+05	\N	monitoring
11	22	2025-10-16 00:28:26.95343+05	2025-10-16 00:28:26.95343+05	\N	monitoring
12	25	2025-10-16 10:46:34.914107+05	2025-10-16 10:46:34.914107+05	\N	monitoring
13	26	2025-10-16 10:48:58.280496+05	2025-10-16 10:48:58.280496+05	\N	monitoring
14	26	2025-10-16 10:48:58.281119+05	2025-10-16 10:48:58.281119+05	\N	reports
15	27	2025-10-16 10:59:30.812308+05	2025-10-16 10:59:30.812308+05	\N	monitoring
16	27	2025-10-16 10:59:30.817277+05	2025-10-16 10:59:30.817277+05	\N	reports
17	28	2025-10-16 11:00:42.387403+05	2025-10-16 11:00:42.387403+05	\N	monitoring
18	28	2025-10-16 11:00:42.389432+05	2025-10-16 11:00:42.389432+05	\N	reports
19	29	2025-10-16 12:16:29.832002+05	2025-10-16 12:16:29.832002+05	\N	monitoring
\.


--
-- Data for Name: user_templates; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.user_templates (id, created_at, updated_at, deleted_at, name, description, role_id, settings, is_active) FROM stdin;
\.


--
-- Data for Name: user_tokens; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.user_tokens (id, created_at, updated_at, deleted_at, user_id, username, token, expires_at, is_active, last_used_at, user_agent, ip_address) FROM stdin;
1	2025-10-16 08:46:18.650089	2025-10-16 08:48:44.13135	\N	23	glomos	5e515a8f2874fc78f31c74af45260333f2c84c35	2025-10-17 08:46:18.649824	f	0001-01-01 00:00:00	curl/8.7.1	::1
2	2025-10-16 08:48:44.13234	2025-10-16 10:16:09.395384	\N	23	glomos	5e515a8f2874fc78f31c74af45260333f2c84c35	2025-10-17 08:48:44.13209	f	0001-01-01 00:00:00	Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 YaBrowser/25.8.0.0 Safari/537.36	::1
3	2025-10-16 10:16:09.400938	2025-10-16 10:16:09.400938	\N	23	glomos	5e515a8f2874fc78f31c74af45260333f2c84c35	2025-10-17 10:16:09.400448	t	0001-01-01 00:00:00	curl/8.7.1	::1
\.


--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.users (id, created_at, updated_at, deleted_at, username, email, password, first_name, last_name, name, phone, telegram_id, is_active, user_type, external_id, external_source, company_id, role_id, template_id, last_login, login_count, axenta_user_type, axenta_user_id, is_axenta_user) FROM stdin;
9	2025-10-15 23:50:59.40934+05	2025-10-15 23:50:59.40934+05	\N	test	test@test.com	$2a$10$by.lunP7Jc0Qq25j9ZYOlu..qbymgh/zXIzxZGtREknXANkfQN2za			test			t	cms_user			1	0	\N	\N	0			f
10	2025-10-15 23:59:17.456524+05	2025-10-15 23:59:17.456524+05	\N	test2	test2@test.com	$2a$10$qWBabJYyPnjQD7sQsqRoP.QL7jUczDVDADp7B/edtNTb4.Tk4xOvm			test2			t	cms_user			1	0	\N	\N	0			f
12	2025-10-16 00:03:05.215197+05	2025-10-16 00:03:05.215197+05	\N	test3	test3@test.com	$2a$10$6Dp/TyK6De8F4d86SiKHh.7XnERzX9WOykucqtBTCcdi1u87JmWRa			test3			t	cms_user			1	0	\N	\N	0			f
16	2025-10-16 00:05:10.650377+05	2025-10-16 00:05:10.650377+05	\N	test4	test4@test.com	$2a$10$84NJRhoXsY0YB.sU50jpp.J.4j..dcn2myEvT2JSX6sCuG7iTX4B2			test4			t	cms_user			1	0	\N	\N	0			f
18	2025-10-16 00:11:24.500119+05	2025-10-16 00:11:24.500119+05	\N	test_sync	test_sync@test.com	$2a$10$yTCo5.kJLaLnyLOEp3vkAObDmpHVqWHORpPUfNerPh8p2C2ZVSnOq			test_sync			t	cms_user			1	0	\N	\N	0			f
20	2025-10-16 00:27:38.196287+05	2025-10-16 00:27:38.196287+05	\N	test_axenta_user	test_axenta@example.com	$2a$10$QtRDnfxBR0hiTi5sw/ZvZul9Y5L6BiYhTUErXtYqT1XlX.78O6g8C			Test User Axenta			t	cms_user			1	0	\N	\N	0			f
21	2025-10-16 00:28:00.804022+05	2025-10-16 00:28:00.804022+05	\N	test_axenta_user2	test_axenta2@example.com	$2a$10$Yw2LY8LJxaaMk7vwdqXQquv6.myH9TXrMl.iuQ60vSenxm/cdvafq			Test User Axenta 2			t	cms_user			1	0	\N	\N	0			f
22	2025-10-16 00:28:26.952688+05	2025-10-16 00:28:26.952688+05	\N	test_axenta_user3	test_axenta3@example.com	$2a$10$CcoK10nVdedpUyovCZwl4uZa21YwXYrhsHsgS2.ouN9kEuhIJtVuy			Test User Axenta 3			t	cms_user			1	0	\N	\N	0			f
24	2025-10-16 00:41:53.70955+05	2025-10-16 00:41:53.70955+05	\N	test_manual	test_manual@example.com	hashed_password	\N	\N	Test Manual User	\N	\N	t	cms_user	\N	\N	1	\N	\N	\N	0	\N	\N	f
1	2025-08-19 23:07:47.752602+05	2025-10-16 01:18:53.847291+05	\N	NEWACRM	com_75@mail.ru	$2a$10$dummy.hash.for.testing	Drew		Drew			t	user	2993	axenta	2	1	\N	\N	0	partner	2993	t
25	2025-10-16 10:46:34.908848+05	2025-10-16 10:46:35.063452+05	\N	test_user_final_success_real_161025	test_user_final_success_real_161025@example.com	$2a$10$fmRAAnECAwJ1z8kUhRIiLu8CSKngFB9XiLT.OLJbjJ.0QmSv7uhkK			Test User Final Success Real			t	cms_user			1	\N	\N	\N	0	local		f
26	2025-10-16 10:48:58.27755+05	2025-10-16 10:48:59.229357+05	\N	success_real_161025@example.com	success_real_161025@example.com	$2a$10$X6u/OY4Y8rklz66PjUacq.pQJAtQNmK/Pl5yNZpnpDxe/gLg0M8D6			success_real_161025@example.com			t	cms_user	16483	axenta	1	\N	\N	\N	0	partner	16483	t
27	2025-10-16 10:59:30.79983+05	2025-10-16 10:59:31.532307+05	\N	1success_real_161025@example.com	1success_real_161025@example.com	$2a$10$gTSl6xu/dMbe2vNeaCcoZ.jncomdY7bht1pCw07wYcj2YmxZaf3hO			1success_real_161025@example.com			t	cms_user	16486	axenta	1	\N	\N	\N	0	partner	16486	t
28	2025-10-16 11:00:42.381471+05	2025-10-16 11:00:43.008308+05	\N	2success_real_161025@example.com	2success_real_161025@example.com	$2a$10$Cxc2yAllbffqfzVhwB.m4.Jd6H5yYLZttIUrFkYOUoZaeHKbRcNVW			2success_real_161025@example.com			t	cms_user	16487	axenta	1	\N	\N	\N	0	partner	16487	t
23	2025-10-16 00:35:05.417432+05	2025-10-16 12:16:09.38098+05	\N	glomos	chudin@glomos.ru	axenta_user	Чудин Андрей Геннадьевич		Чудин Андрей Геннадьевич			t	user	215	axenta	0	1	\N	\N	0	partner	215	t
29	2025-10-16 12:16:29.826631+05	2025-10-16 12:16:29.99881+05	\N	test_user_after_migration_161025	test_user_after_migration_161025@example.com	$2a$10$/EpfGacAtP9qK4yEYMBvneoM/taGd52FsRuBnZRajUI1dRqTE9OQu			Test User After Migration			t	cms_user			1	\N	\N	\N	0	local		f
\.


--
-- Data for Name: warehouse_operations; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.warehouse_operations (id, created_at, updated_at, deleted_at, type, description, status, equipment_id, quantity, from_location, to_location, user_id, document_number, notes, installation_id, company_id) FROM stdin;
\.


--
-- Data for Name: billing_plans; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.billing_plans (id, created_at, updated_at, deleted_at, name, description, price, currency, billing_period, max_devices, max_users, max_storage, has_analytics, has_api, has_support, has_custom_domain, is_active, is_popular, company_id) FROM stdin;
\.


--
-- Data for Name: companies; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.companies (id, created_at, updated_at, deleted_at, name, database_schema, domain, axetna_login, axetna_password, bitrix24_webhook_url, bitrix24_client_id, bitrix24_client_secret, contact_email, contact_phone, contact_person, address, city, country, is_active, max_users, max_objects, storage_quota, language, timezone, currency, hierarchy, subscription_id) FROM stdin;
\.


--
-- Data for Name: contracts; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.contracts (id, created_at, updated_at, deleted_at, number, title, description, company_id, client_name, client_inn, client_kpp, client_email, client_phone, client_address, start_date, end_date, signed_at, tariff_plan_id, total_amount, currency, status, is_active, notify_before, notes, external_id) FROM stdin;
\.


--
-- Data for Name: equipment_categories; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.equipment_categories (id, created_at, updated_at, deleted_at, name, description, code, min_stock_level, is_active) FROM stdin;
\.


--
-- Data for Name: installers; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.installers (id, created_at, updated_at, deleted_at, first_name, last_name, middle_name, type, phone, email, telegram_id, specialization, skill_level, experience, location_ids, max_daily_installations, working_hours_start, working_hours_end, working_days, hourly_rate, is_active, status, last_worked_at, rating, completed_jobs, notes) FROM stdin;
\.


--
-- Data for Name: locations; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.locations (id, created_at, updated_at, deleted_at, city, region, country, latitude, longitude, timezone, is_active, notes) FROM stdin;
\.


--
-- Data for Name: monitoring_notification_templates; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.monitoring_notification_templates (id, created_at, updated_at, deleted_at, name, description, type, event_type, email_subject, email_body, sms_message, telegram_message, webhook_payload, priority, retry_count, retry_interval, max_per_hour, max_per_day, active_from, active_until, week_days, time_from, time_until, is_active, usage_count, variables) FROM stdin;
\.


--
-- Data for Name: monitoring_templates; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.monitoring_templates (id, created_at, updated_at, deleted_at, name, description, check_interval, alert_threshold, geo_fence_enabled, speed_limit, notify_on_offline, notify_on_move, notify_on_speed, notify_on_geo_fence, email_enabled, sms_enabled, telegram_enabled, webhook_enabled, settings, is_active, usage_count) FROM stdin;
\.


--
-- Data for Name: object_templates; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.object_templates (id, created_at, updated_at, deleted_at, name, description, category, icon, color, config, default_settings, required_equipment, is_active, is_system, usage_count) FROM stdin;
\.


--
-- Data for Name: objects; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.objects (id, created_at, updated_at, deleted_at, name, type, description, latitude, longitude, address, imei, phone_number, serial_number, status, is_active, scheduled_delete_at, last_activity_at, company_id, contract_id, template_id, location_id, settings, tags, notes, external_id) FROM stdin;
\.


--
-- Data for Name: permissions; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.permissions (id, created_at, updated_at, deleted_at, name, display_name, description, resource, action, category, is_active) FROM stdin;
\.


--
-- Data for Name: role_permissions; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.role_permissions (role_id, permission_id) FROM stdin;
\.


--
-- Data for Name: roles; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.roles (id, created_at, updated_at, deleted_at, name, display_name, description, color, priority, is_active, is_system) FROM stdin;
\.


--
-- Data for Name: tariff_plans; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.tariff_plans (id, created_at, updated_at, deleted_at, name, description, price, currency, billing_period, max_devices, max_users, max_storage, has_analytics, has_api, has_support, has_custom_domain, is_active, is_popular, company_id, setup_fee, minimum_period, discount_percent, is_promotional, promotional_until, price_per_object, free_objects_count, inactive_price_ratio) FROM stdin;
\.


--
-- Data for Name: user_templates; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.user_templates (id, created_at, updated_at, deleted_at, name, description, role_id, settings, is_active) FROM stdin;
\.


--
-- Data for Name: users; Type: TABLE DATA; Schema: tenant_default; Owner: postgres
--

COPY tenant_default.users (id, created_at, updated_at, deleted_at, username, email, password, first_name, last_name, name, phone, telegram_id, is_active, user_type, external_id, external_source, axenta_user_type, axenta_user_id, is_axenta_user, company_id, role_id, template_id, last_login, login_count) FROM stdin;
\.


--
-- Data for Name: equipment; Type: TABLE DATA; Schema: tenant_newacrm; Owner: postgres
--

COPY tenant_newacrm.equipment (id, created_at, updated_at, deleted_at, type, model, serial_number, status, company_id) FROM stdin;
\.


--
-- Data for Name: installations; Type: TABLE DATA; Schema: tenant_newacrm; Owner: postgres
--

COPY tenant_newacrm.installations (id, created_at, updated_at, deleted_at, type, status, priority, description, scheduled_at, estimated_duration, object_id, installer_id, location_id, client_contact, address, notes, company_id) FROM stdin;
\.


--
-- Data for Name: installers; Type: TABLE DATA; Schema: tenant_newacrm; Owner: postgres
--

COPY tenant_newacrm.installers (id, created_at, updated_at, deleted_at, first_name, last_name, phone, email, type, is_active, company_id) FROM stdin;
\.


--
-- Data for Name: locations; Type: TABLE DATA; Schema: tenant_newacrm; Owner: postgres
--

COPY tenant_newacrm.locations (id, created_at, updated_at, deleted_at, city, region, country, is_active, company_id) FROM stdin;
\.


--
-- Name: billing_plans_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.billing_plans_id_seq', 1, false);


--
-- Name: companies_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.companies_id_seq', 6, true);


--
-- Name: contract_appendices_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.contract_appendices_id_seq', 1, false);


--
-- Name: contracts_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.contracts_id_seq', 1, false);


--
-- Name: equipment_categories_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.equipment_categories_id_seq', 1, false);


--
-- Name: equipment_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.equipment_id_seq', 1, false);


--
-- Name: installations_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.installations_id_seq', 1, false);


--
-- Name: installers_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.installers_id_seq', 1, false);


--
-- Name: integration_errors_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.integration_errors_id_seq', 1, false);


--
-- Name: integrations_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.integrations_id_seq', 1, false);


--
-- Name: local_users_id_seq; Type: SEQUENCE SET; Schema: public; Owner: com
--

SELECT pg_catalog.setval('public.local_users_id_seq', 1, true);


--
-- Name: locations_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.locations_id_seq', 1, false);


--
-- Name: notification_logs_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.notification_logs_id_seq', 1, false);


--
-- Name: notification_settings_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.notification_settings_id_seq', 1, false);


--
-- Name: notification_templates_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.notification_templates_id_seq', 1, false);


--
-- Name: object_templates_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.object_templates_id_seq', 1, false);


--
-- Name: objects_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.objects_id_seq', 1, false);


--
-- Name: permissions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.permissions_id_seq', 1, false);


--
-- Name: refresh_tokens_id_seq; Type: SEQUENCE SET; Schema: public; Owner: com
--

SELECT pg_catalog.setval('public.refresh_tokens_id_seq', 12, true);


--
-- Name: report_executions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.report_executions_id_seq', 1, false);


--
-- Name: report_schedules_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.report_schedules_id_seq', 1, false);


--
-- Name: report_templates_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.report_templates_id_seq', 1, false);


--
-- Name: reports_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.reports_id_seq', 1, false);


--
-- Name: roles_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.roles_id_seq', 3, true);


--
-- Name: stock_alerts_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.stock_alerts_id_seq', 1, false);


--
-- Name: subscriptions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.subscriptions_id_seq', 1, false);


--
-- Name: user_accesses_id_seq; Type: SEQUENCE SET; Schema: public; Owner: com
--

SELECT pg_catalog.setval('public.user_accesses_id_seq', 26, true);


--
-- Name: user_notification_preferences_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.user_notification_preferences_id_seq', 1, false);


--
-- Name: user_tabs_id_seq; Type: SEQUENCE SET; Schema: public; Owner: com
--

SELECT pg_catalog.setval('public.user_tabs_id_seq', 19, true);


--
-- Name: user_templates_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.user_templates_id_seq', 1, false);


--
-- Name: user_tokens_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.user_tokens_id_seq', 3, true);


--
-- Name: users_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.users_id_seq', 29, true);


--
-- Name: warehouse_operations_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.warehouse_operations_id_seq', 1, false);


--
-- Name: billing_plans_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.billing_plans_id_seq', 1, false);


--
-- Name: companies_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.companies_id_seq', 1, false);


--
-- Name: contracts_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.contracts_id_seq', 1, false);


--
-- Name: equipment_categories_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.equipment_categories_id_seq', 1, false);


--
-- Name: installers_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.installers_id_seq', 1, false);


--
-- Name: locations_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.locations_id_seq', 1, false);


--
-- Name: monitoring_notification_templates_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.monitoring_notification_templates_id_seq', 1, false);


--
-- Name: monitoring_templates_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.monitoring_templates_id_seq', 1, false);


--
-- Name: object_templates_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.object_templates_id_seq', 1, false);


--
-- Name: objects_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.objects_id_seq', 1, false);


--
-- Name: permissions_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.permissions_id_seq', 1, false);


--
-- Name: roles_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.roles_id_seq', 1, false);


--
-- Name: tariff_plans_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.tariff_plans_id_seq', 1, false);


--
-- Name: user_templates_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.user_templates_id_seq', 1, false);


--
-- Name: users_id_seq; Type: SEQUENCE SET; Schema: tenant_default; Owner: postgres
--

SELECT pg_catalog.setval('tenant_default.users_id_seq', 1, false);


--
-- Name: equipment_id_seq; Type: SEQUENCE SET; Schema: tenant_newacrm; Owner: postgres
--

SELECT pg_catalog.setval('tenant_newacrm.equipment_id_seq', 1, false);


--
-- Name: installations_id_seq; Type: SEQUENCE SET; Schema: tenant_newacrm; Owner: postgres
--

SELECT pg_catalog.setval('tenant_newacrm.installations_id_seq', 1, false);


--
-- Name: installers_id_seq; Type: SEQUENCE SET; Schema: tenant_newacrm; Owner: postgres
--

SELECT pg_catalog.setval('tenant_newacrm.installers_id_seq', 1, false);


--
-- Name: locations_id_seq; Type: SEQUENCE SET; Schema: tenant_newacrm; Owner: postgres
--

SELECT pg_catalog.setval('tenant_newacrm.locations_id_seq', 1, false);


--
-- Name: billing_plans billing_plans_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.billing_plans
    ADD CONSTRAINT billing_plans_pkey PRIMARY KEY (id);


--
-- Name: companies companies_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.companies
    ADD CONSTRAINT companies_pkey PRIMARY KEY (id);


--
-- Name: contract_appendices contract_appendices_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.contract_appendices
    ADD CONSTRAINT contract_appendices_pkey PRIMARY KEY (id);


--
-- Name: contracts contracts_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.contracts
    ADD CONSTRAINT contracts_pkey PRIMARY KEY (id);


--
-- Name: equipment_categories equipment_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.equipment_categories
    ADD CONSTRAINT equipment_categories_pkey PRIMARY KEY (id);


--
-- Name: equipment equipment_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.equipment
    ADD CONSTRAINT equipment_pkey PRIMARY KEY (id);


--
-- Name: installation_equipment installation_equipment_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installation_equipment
    ADD CONSTRAINT installation_equipment_pkey PRIMARY KEY (equipment_id, installation_id);


--
-- Name: installations installations_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installations
    ADD CONSTRAINT installations_pkey PRIMARY KEY (id);


--
-- Name: installer_locations installer_locations_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installer_locations
    ADD CONSTRAINT installer_locations_pkey PRIMARY KEY (installer_id, location_id);


--
-- Name: installers installers_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installers
    ADD CONSTRAINT installers_pkey PRIMARY KEY (id);


--
-- Name: integration_errors integration_errors_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.integration_errors
    ADD CONSTRAINT integration_errors_pkey PRIMARY KEY (id);


--
-- Name: integrations integrations_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.integrations
    ADD CONSTRAINT integrations_pkey PRIMARY KEY (id);


--
-- Name: local_users local_users_pkey; Type: CONSTRAINT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.local_users
    ADD CONSTRAINT local_users_pkey PRIMARY KEY (id);


--
-- Name: local_users local_users_username_key; Type: CONSTRAINT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.local_users
    ADD CONSTRAINT local_users_username_key UNIQUE (username);


--
-- Name: locations locations_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.locations
    ADD CONSTRAINT locations_pkey PRIMARY KEY (id);


--
-- Name: notification_logs notification_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.notification_logs
    ADD CONSTRAINT notification_logs_pkey PRIMARY KEY (id);


--
-- Name: notification_settings notification_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.notification_settings
    ADD CONSTRAINT notification_settings_pkey PRIMARY KEY (id);


--
-- Name: notification_templates notification_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.notification_templates
    ADD CONSTRAINT notification_templates_pkey PRIMARY KEY (id);


--
-- Name: object_templates object_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.object_templates
    ADD CONSTRAINT object_templates_pkey PRIMARY KEY (id);


--
-- Name: objects objects_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.objects
    ADD CONSTRAINT objects_pkey PRIMARY KEY (id);


--
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- Name: refresh_tokens refresh_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (id);


--
-- Name: refresh_tokens refresh_tokens_token_key; Type: CONSTRAINT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_token_key UNIQUE (token);


--
-- Name: report_executions report_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_executions
    ADD CONSTRAINT report_executions_pkey PRIMARY KEY (id);


--
-- Name: report_schedules report_schedules_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_schedules
    ADD CONSTRAINT report_schedules_pkey PRIMARY KEY (id);


--
-- Name: report_templates report_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_templates
    ADD CONSTRAINT report_templates_pkey PRIMARY KEY (id);


--
-- Name: reports reports_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.reports
    ADD CONSTRAINT reports_pkey PRIMARY KEY (id);


--
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: stock_alerts stock_alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_alerts
    ADD CONSTRAINT stock_alerts_pkey PRIMARY KEY (id);


--
-- Name: subscriptions subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);


--
-- Name: user_accesses user_accesses_pkey; Type: CONSTRAINT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.user_accesses
    ADD CONSTRAINT user_accesses_pkey PRIMARY KEY (id);


--
-- Name: user_notification_preferences user_notification_preferences_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_notification_preferences
    ADD CONSTRAINT user_notification_preferences_pkey PRIMARY KEY (id);


--
-- Name: user_tabs user_tabs_pkey; Type: CONSTRAINT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.user_tabs
    ADD CONSTRAINT user_tabs_pkey PRIMARY KEY (id);


--
-- Name: user_templates user_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_templates
    ADD CONSTRAINT user_templates_pkey PRIMARY KEY (id);


--
-- Name: user_tokens user_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_tokens
    ADD CONSTRAINT user_tokens_pkey PRIMARY KEY (id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: warehouse_operations warehouse_operations_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.warehouse_operations
    ADD CONSTRAINT warehouse_operations_pkey PRIMARY KEY (id);


--
-- Name: billing_plans billing_plans_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.billing_plans
    ADD CONSTRAINT billing_plans_pkey PRIMARY KEY (id);


--
-- Name: companies companies_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.companies
    ADD CONSTRAINT companies_pkey PRIMARY KEY (id);


--
-- Name: contracts contracts_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.contracts
    ADD CONSTRAINT contracts_pkey PRIMARY KEY (id);


--
-- Name: equipment_categories equipment_categories_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.equipment_categories
    ADD CONSTRAINT equipment_categories_pkey PRIMARY KEY (id);


--
-- Name: installers installers_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.installers
    ADD CONSTRAINT installers_pkey PRIMARY KEY (id);


--
-- Name: locations locations_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.locations
    ADD CONSTRAINT locations_pkey PRIMARY KEY (id);


--
-- Name: monitoring_notification_templates monitoring_notification_templates_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.monitoring_notification_templates
    ADD CONSTRAINT monitoring_notification_templates_pkey PRIMARY KEY (id);


--
-- Name: monitoring_templates monitoring_templates_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.monitoring_templates
    ADD CONSTRAINT monitoring_templates_pkey PRIMARY KEY (id);


--
-- Name: object_templates object_templates_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.object_templates
    ADD CONSTRAINT object_templates_pkey PRIMARY KEY (id);


--
-- Name: objects objects_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.objects
    ADD CONSTRAINT objects_pkey PRIMARY KEY (id);


--
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: tariff_plans tariff_plans_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.tariff_plans
    ADD CONSTRAINT tariff_plans_pkey PRIMARY KEY (id);


--
-- Name: user_templates user_templates_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.user_templates
    ADD CONSTRAINT user_templates_pkey PRIMARY KEY (id);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- Name: equipment equipment_pkey; Type: CONSTRAINT; Schema: tenant_newacrm; Owner: postgres
--

ALTER TABLE ONLY tenant_newacrm.equipment
    ADD CONSTRAINT equipment_pkey PRIMARY KEY (id);


--
-- Name: installations installations_pkey; Type: CONSTRAINT; Schema: tenant_newacrm; Owner: postgres
--

ALTER TABLE ONLY tenant_newacrm.installations
    ADD CONSTRAINT installations_pkey PRIMARY KEY (id);


--
-- Name: installers installers_pkey; Type: CONSTRAINT; Schema: tenant_newacrm; Owner: postgres
--

ALTER TABLE ONLY tenant_newacrm.installers
    ADD CONSTRAINT installers_pkey PRIMARY KEY (id);


--
-- Name: locations locations_pkey; Type: CONSTRAINT; Schema: tenant_newacrm; Owner: postgres
--

ALTER TABLE ONLY tenant_newacrm.locations
    ADD CONSTRAINT locations_pkey PRIMARY KEY (id);


--
-- Name: idx_billing_plans_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_billing_plans_company_id ON public.billing_plans USING btree (company_id);


--
-- Name: idx_billing_plans_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_billing_plans_deleted_at ON public.billing_plans USING btree (deleted_at);


--
-- Name: idx_billing_plans_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_billing_plans_name ON public.billing_plans USING btree (name);


--
-- Name: idx_companies_database_schema; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_companies_database_schema ON public.companies USING btree (database_schema);


--
-- Name: idx_companies_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_companies_deleted_at ON public.companies USING btree (deleted_at);


--
-- Name: idx_companies_domain; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_companies_domain ON public.companies USING btree (domain);


--
-- Name: idx_contract_appendices_contract_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_contract_appendices_contract_id ON public.contract_appendices USING btree (contract_id);


--
-- Name: idx_contract_appendices_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_contract_appendices_deleted_at ON public.contract_appendices USING btree (deleted_at);


--
-- Name: idx_contracts_client_inn; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_contracts_client_inn ON public.contracts USING btree (client_inn);


--
-- Name: idx_contracts_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_contracts_company_id ON public.contracts USING btree (company_id);


--
-- Name: idx_contracts_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_contracts_deleted_at ON public.contracts USING btree (deleted_at);


--
-- Name: idx_contracts_end_date; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_contracts_end_date ON public.contracts USING btree (end_date);


--
-- Name: idx_contracts_expiring; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_contracts_expiring ON public.contracts USING btree (end_date);


--
-- Name: idx_contracts_number; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_contracts_number ON public.contracts USING btree (number);


--
-- Name: idx_equipment_categories_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_equipment_categories_code ON public.equipment_categories USING btree (code);


--
-- Name: idx_equipment_categories_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_equipment_categories_deleted_at ON public.equipment_categories USING btree (deleted_at);


--
-- Name: idx_equipment_categories_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_equipment_categories_name ON public.equipment_categories USING btree (name);


--
-- Name: idx_equipment_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_equipment_deleted_at ON public.equipment USING btree (deleted_at);


--
-- Name: idx_equipment_imei; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_equipment_imei ON public.equipment USING btree (imei);


--
-- Name: idx_equipment_qr_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_equipment_qr_code ON public.equipment USING btree (qr_code);


--
-- Name: idx_equipment_serial; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_equipment_serial ON public.equipment USING btree (serial_number);


--
-- Name: idx_equipment_serial_number; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_equipment_serial_number ON public.equipment USING btree (serial_number);


--
-- Name: idx_installations_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_installations_company_id ON public.installations USING btree (company_id);


--
-- Name: idx_installations_created_by_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_installations_created_by_user_id ON public.installations USING btree (created_by_user_id);


--
-- Name: idx_installations_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_installations_deleted_at ON public.installations USING btree (deleted_at);


--
-- Name: idx_installations_installer; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_installations_installer ON public.installations USING btree (installer_id);


--
-- Name: idx_installations_installer_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_installations_installer_id ON public.installations USING btree (installer_id);


--
-- Name: idx_installations_location_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_installations_location_id ON public.installations USING btree (location_id);


--
-- Name: idx_installations_object; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_installations_object ON public.installations USING btree (object_id);


--
-- Name: idx_installations_object_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_installations_object_id ON public.installations USING btree (object_id);


--
-- Name: idx_installers_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_installers_deleted_at ON public.installers USING btree (deleted_at);


--
-- Name: idx_installers_email; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_installers_email ON public.installers USING btree (email);


--
-- Name: idx_integration_errors_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_integration_errors_deleted_at ON public.integration_errors USING btree (deleted_at);


--
-- Name: idx_integration_errors_external_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_integration_errors_external_id ON public.integration_errors USING btree (external_id);


--
-- Name: idx_integration_errors_object_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_integration_errors_object_id ON public.integration_errors USING btree (object_id);


--
-- Name: idx_integration_errors_tenant_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_integration_errors_tenant_id ON public.integration_errors USING btree (tenant_id);


--
-- Name: idx_integrations_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_integrations_company_id ON public.integrations USING btree (company_id);


--
-- Name: idx_integrations_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_integrations_deleted_at ON public.integrations USING btree (deleted_at);


--
-- Name: idx_local_users_company_id; Type: INDEX; Schema: public; Owner: com
--

CREATE INDEX idx_local_users_company_id ON public.local_users USING btree (company_id);


--
-- Name: idx_local_users_deleted_at; Type: INDEX; Schema: public; Owner: com
--

CREATE INDEX idx_local_users_deleted_at ON public.local_users USING btree (deleted_at);


--
-- Name: idx_local_users_email; Type: INDEX; Schema: public; Owner: com
--

CREATE INDEX idx_local_users_email ON public.local_users USING btree (email);


--
-- Name: idx_local_users_username; Type: INDEX; Schema: public; Owner: com
--

CREATE INDEX idx_local_users_username ON public.local_users USING btree (username);


--
-- Name: idx_locations_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_locations_deleted_at ON public.locations USING btree (deleted_at);


--
-- Name: idx_notification_logs_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_notification_logs_company_id ON public.notification_logs USING btree (company_id);


--
-- Name: idx_notification_logs_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_notification_logs_deleted_at ON public.notification_logs USING btree (deleted_at);


--
-- Name: idx_notification_logs_template_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_notification_logs_template_id ON public.notification_logs USING btree (template_id);


--
-- Name: idx_notification_logs_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_notification_logs_user_id ON public.notification_logs USING btree (user_id);


--
-- Name: idx_notification_settings_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_notification_settings_company_id ON public.notification_settings USING btree (company_id);


--
-- Name: idx_notification_settings_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_notification_settings_deleted_at ON public.notification_settings USING btree (deleted_at);


--
-- Name: idx_notification_templates_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_notification_templates_company_id ON public.notification_templates USING btree (company_id);


--
-- Name: idx_notification_templates_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_notification_templates_deleted_at ON public.notification_templates USING btree (deleted_at);


--
-- Name: idx_notification_templates_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_notification_templates_name ON public.notification_templates USING btree (name);


--
-- Name: idx_object_templates_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_object_templates_deleted_at ON public.object_templates USING btree (deleted_at);


--
-- Name: idx_objects_contract; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_objects_contract ON public.objects USING btree (contract_id);


--
-- Name: idx_objects_contract_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_objects_contract_id ON public.objects USING btree (contract_id);


--
-- Name: idx_objects_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_objects_deleted_at ON public.objects USING btree (deleted_at);


--
-- Name: idx_objects_fulltext; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_objects_fulltext ON public.objects USING gin (to_tsvector('russian'::regconfig, (((COALESCE(name, ''::character varying))::text || ' '::text) || COALESCE(description, ''::text))));


--
-- Name: idx_objects_imei; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_objects_imei ON public.objects USING btree (imei);


--
-- Name: idx_objects_location_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_objects_location_id ON public.objects USING btree (location_id);


--
-- Name: idx_objects_phone; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_objects_phone ON public.objects USING btree (phone_number);


--
-- Name: idx_objects_scheduled_delete; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_objects_scheduled_delete ON public.objects USING btree (scheduled_delete_at);


--
-- Name: idx_permissions_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_permissions_deleted_at ON public.permissions USING btree (deleted_at);


--
-- Name: idx_permissions_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_permissions_name ON public.permissions USING btree (name);


--
-- Name: idx_refresh_tokens_token; Type: INDEX; Schema: public; Owner: com
--

CREATE INDEX idx_refresh_tokens_token ON public.refresh_tokens USING btree (token);


--
-- Name: idx_refresh_tokens_user_id; Type: INDEX; Schema: public; Owner: com
--

CREATE INDEX idx_refresh_tokens_user_id ON public.refresh_tokens USING btree (user_id);


--
-- Name: idx_report_executions_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_report_executions_company_id ON public.report_executions USING btree (company_id);


--
-- Name: idx_report_executions_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_report_executions_deleted_at ON public.report_executions USING btree (deleted_at);


--
-- Name: idx_report_executions_schedule_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_report_executions_schedule_id ON public.report_executions USING btree (schedule_id);


--
-- Name: idx_report_schedules_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_report_schedules_company_id ON public.report_schedules USING btree (company_id);


--
-- Name: idx_report_schedules_created_by_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_report_schedules_created_by_id ON public.report_schedules USING btree (created_by_id);


--
-- Name: idx_report_schedules_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_report_schedules_deleted_at ON public.report_schedules USING btree (deleted_at);


--
-- Name: idx_report_templates_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_report_templates_company_id ON public.report_templates USING btree (company_id);


--
-- Name: idx_report_templates_created_by_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_report_templates_created_by_id ON public.report_templates USING btree (created_by_id);


--
-- Name: idx_report_templates_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_report_templates_deleted_at ON public.report_templates USING btree (deleted_at);


--
-- Name: idx_reports_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_reports_company_id ON public.reports USING btree (company_id);


--
-- Name: idx_reports_created_by_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_reports_created_by_id ON public.reports USING btree (created_by_id);


--
-- Name: idx_reports_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_reports_deleted_at ON public.reports USING btree (deleted_at);


--
-- Name: idx_reports_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_reports_status ON public.reports USING btree (status);


--
-- Name: idx_reports_status_created; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_reports_status_created ON public.reports USING btree (status, created_at);


--
-- Name: idx_roles_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_roles_deleted_at ON public.roles USING btree (deleted_at);


--
-- Name: idx_roles_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_roles_name ON public.roles USING btree (name);


--
-- Name: idx_stock_alerts_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_alerts_company_id ON public.stock_alerts USING btree (company_id);


--
-- Name: idx_stock_alerts_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_alerts_deleted_at ON public.stock_alerts USING btree (deleted_at);


--
-- Name: idx_subscriptions_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_subscriptions_company_id ON public.subscriptions USING btree (company_id);


--
-- Name: idx_subscriptions_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_subscriptions_deleted_at ON public.subscriptions USING btree (deleted_at);


--
-- Name: idx_user_notification_preferences_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_user_notification_preferences_company_id ON public.user_notification_preferences USING btree (company_id);


--
-- Name: idx_user_notification_preferences_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_user_notification_preferences_deleted_at ON public.user_notification_preferences USING btree (deleted_at);


--
-- Name: idx_user_notification_preferences_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_user_notification_preferences_user_id ON public.user_notification_preferences USING btree (user_id);


--
-- Name: idx_user_templates_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_user_templates_deleted_at ON public.user_templates USING btree (deleted_at);


--
-- Name: idx_users_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_company_id ON public.users USING btree (company_id);


--
-- Name: idx_users_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_deleted_at ON public.users USING btree (deleted_at);


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_users_email ON public.users USING btree (email);


--
-- Name: idx_users_fulltext; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_fulltext ON public.users USING gin (to_tsvector('russian'::regconfig, first_name));


--
-- Name: idx_users_role_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_role_id ON public.users USING btree (role_id);


--
-- Name: idx_users_username; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_users_username ON public.users USING btree (username);


--
-- Name: idx_warehouse_operations_company_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_warehouse_operations_company_id ON public.warehouse_operations USING btree (company_id);


--
-- Name: idx_warehouse_operations_deleted_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_warehouse_operations_deleted_at ON public.warehouse_operations USING btree (deleted_at);


--
-- Name: idx_warehouse_operations_equipment_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_warehouse_operations_equipment_id ON public.warehouse_operations USING btree (equipment_id);


--
-- Name: idx_warehouse_operations_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_warehouse_operations_user_id ON public.warehouse_operations USING btree (user_id);


--
-- Name: idx_billing_plans_company_id; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_billing_plans_company_id ON tenant_default.billing_plans USING btree (company_id);


--
-- Name: idx_billing_plans_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_billing_plans_deleted_at ON tenant_default.billing_plans USING btree (deleted_at);


--
-- Name: idx_billing_plans_name; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_billing_plans_name ON tenant_default.billing_plans USING btree (name);


--
-- Name: idx_companies_database_schema; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_companies_database_schema ON tenant_default.companies USING btree (database_schema);


--
-- Name: idx_companies_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_companies_deleted_at ON tenant_default.companies USING btree (deleted_at);


--
-- Name: idx_companies_domain; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_companies_domain ON tenant_default.companies USING btree (domain);


--
-- Name: idx_contracts_company_id; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_contracts_company_id ON tenant_default.contracts USING btree (company_id);


--
-- Name: idx_contracts_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_contracts_deleted_at ON tenant_default.contracts USING btree (deleted_at);


--
-- Name: idx_contracts_number; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_contracts_number ON tenant_default.contracts USING btree (number);


--
-- Name: idx_equipment_categories_code; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_equipment_categories_code ON tenant_default.equipment_categories USING btree (code);


--
-- Name: idx_equipment_categories_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_equipment_categories_deleted_at ON tenant_default.equipment_categories USING btree (deleted_at);


--
-- Name: idx_equipment_categories_name; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_equipment_categories_name ON tenant_default.equipment_categories USING btree (name);


--
-- Name: idx_installers_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_installers_deleted_at ON tenant_default.installers USING btree (deleted_at);


--
-- Name: idx_installers_email; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_installers_email ON tenant_default.installers USING btree (email);


--
-- Name: idx_locations_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_locations_deleted_at ON tenant_default.locations USING btree (deleted_at);


--
-- Name: idx_monitoring_notification_templates_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_monitoring_notification_templates_deleted_at ON tenant_default.monitoring_notification_templates USING btree (deleted_at);


--
-- Name: idx_monitoring_templates_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_monitoring_templates_deleted_at ON tenant_default.monitoring_templates USING btree (deleted_at);


--
-- Name: idx_object_templates_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_object_templates_deleted_at ON tenant_default.object_templates USING btree (deleted_at);


--
-- Name: idx_objects_company_id; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_objects_company_id ON tenant_default.objects USING btree (company_id);


--
-- Name: idx_objects_contract_id; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_objects_contract_id ON tenant_default.objects USING btree (contract_id);


--
-- Name: idx_objects_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_objects_deleted_at ON tenant_default.objects USING btree (deleted_at);


--
-- Name: idx_objects_imei; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_objects_imei ON tenant_default.objects USING btree (imei);


--
-- Name: idx_objects_location_id; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_objects_location_id ON tenant_default.objects USING btree (location_id);


--
-- Name: idx_permissions_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_permissions_deleted_at ON tenant_default.permissions USING btree (deleted_at);


--
-- Name: idx_permissions_name; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_permissions_name ON tenant_default.permissions USING btree (name);


--
-- Name: idx_roles_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_roles_deleted_at ON tenant_default.roles USING btree (deleted_at);


--
-- Name: idx_roles_name; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_roles_name ON tenant_default.roles USING btree (name);


--
-- Name: idx_tariff_plans_company_id; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_tariff_plans_company_id ON tenant_default.tariff_plans USING btree (company_id);


--
-- Name: idx_tariff_plans_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_tariff_plans_deleted_at ON tenant_default.tariff_plans USING btree (deleted_at);


--
-- Name: idx_tariff_plans_name; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE UNIQUE INDEX idx_tariff_plans_name ON tenant_default.tariff_plans USING btree (name);


--
-- Name: idx_user_templates_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_user_templates_deleted_at ON tenant_default.user_templates USING btree (deleted_at);


--
-- Name: idx_users_company_id; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_users_company_id ON tenant_default.users USING btree (company_id);


--
-- Name: idx_users_deleted_at; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_users_deleted_at ON tenant_default.users USING btree (deleted_at);


--
-- Name: idx_users_role_id; Type: INDEX; Schema: tenant_default; Owner: postgres
--

CREATE INDEX idx_users_role_id ON tenant_default.users USING btree (role_id);


--
-- Name: contract_appendices fk_contracts_appendices; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.contract_appendices
    ADD CONSTRAINT fk_contracts_appendices FOREIGN KEY (contract_id) REFERENCES public.contracts(id);


--
-- Name: objects fk_contracts_objects; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.objects
    ADD CONSTRAINT fk_contracts_objects FOREIGN KEY (contract_id) REFERENCES public.contracts(id);


--
-- Name: equipment fk_equipment_categories_equipment; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.equipment
    ADD CONSTRAINT fk_equipment_categories_equipment FOREIGN KEY (category_id) REFERENCES public.equipment_categories(id);


--
-- Name: installation_equipment fk_installation_equipment_equipment; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installation_equipment
    ADD CONSTRAINT fk_installation_equipment_equipment FOREIGN KEY (equipment_id) REFERENCES public.equipment(id);


--
-- Name: installation_equipment fk_installation_equipment_installation; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installation_equipment
    ADD CONSTRAINT fk_installation_equipment_installation FOREIGN KEY (installation_id) REFERENCES public.installations(id);


--
-- Name: installations fk_installations_created_by_user; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installations
    ADD CONSTRAINT fk_installations_created_by_user FOREIGN KEY (created_by_user_id) REFERENCES public.users(id);


--
-- Name: installations fk_installations_location; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installations
    ADD CONSTRAINT fk_installations_location FOREIGN KEY (location_id) REFERENCES public.locations(id);


--
-- Name: installer_locations fk_installer_locations_installer; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installer_locations
    ADD CONSTRAINT fk_installer_locations_installer FOREIGN KEY (installer_id) REFERENCES public.installers(id);


--
-- Name: installer_locations fk_installer_locations_location; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installer_locations
    ADD CONSTRAINT fk_installer_locations_location FOREIGN KEY (location_id) REFERENCES public.locations(id);


--
-- Name: installations fk_installers_installations; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installations
    ADD CONSTRAINT fk_installers_installations FOREIGN KEY (installer_id) REFERENCES public.installers(id);


--
-- Name: objects fk_locations_objects; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.objects
    ADD CONSTRAINT fk_locations_objects FOREIGN KEY (location_id) REFERENCES public.locations(id);


--
-- Name: notification_logs fk_notification_logs_template; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.notification_logs
    ADD CONSTRAINT fk_notification_logs_template FOREIGN KEY (template_id) REFERENCES public.notification_templates(id);


--
-- Name: notification_logs fk_notification_logs_user; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.notification_logs
    ADD CONSTRAINT fk_notification_logs_user FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: objects fk_object_templates_objects; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.objects
    ADD CONSTRAINT fk_object_templates_objects FOREIGN KEY (template_id) REFERENCES public.object_templates(id);


--
-- Name: equipment fk_objects_equipment; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.equipment
    ADD CONSTRAINT fk_objects_equipment FOREIGN KEY (object_id) REFERENCES public.objects(id);


--
-- Name: installations fk_objects_installations; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.installations
    ADD CONSTRAINT fk_objects_installations FOREIGN KEY (object_id) REFERENCES public.objects(id);


--
-- Name: report_executions fk_report_executions_report; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_executions
    ADD CONSTRAINT fk_report_executions_report FOREIGN KEY (report_id) REFERENCES public.reports(id);


--
-- Name: report_executions fk_report_executions_schedule; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_executions
    ADD CONSTRAINT fk_report_executions_schedule FOREIGN KEY (schedule_id) REFERENCES public.report_schedules(id);


--
-- Name: report_schedules fk_report_schedules_created_by; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_schedules
    ADD CONSTRAINT fk_report_schedules_created_by FOREIGN KEY (created_by_id) REFERENCES public.users(id);


--
-- Name: report_schedules fk_report_schedules_last_report; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_schedules
    ADD CONSTRAINT fk_report_schedules_last_report FOREIGN KEY (last_report_id) REFERENCES public.reports(id);


--
-- Name: report_schedules fk_report_schedules_template; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_schedules
    ADD CONSTRAINT fk_report_schedules_template FOREIGN KEY (template_id) REFERENCES public.report_templates(id);


--
-- Name: report_templates fk_report_templates_created_by; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.report_templates
    ADD CONSTRAINT fk_report_templates_created_by FOREIGN KEY (created_by_id) REFERENCES public.users(id);


--
-- Name: reports fk_reports_created_by; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.reports
    ADD CONSTRAINT fk_reports_created_by FOREIGN KEY (created_by_id) REFERENCES public.users(id);


--
-- Name: role_permissions fk_role_permissions_permission; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT fk_role_permissions_permission FOREIGN KEY (permission_id) REFERENCES public.permissions(id);


--
-- Name: role_permissions fk_role_permissions_role; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT fk_role_permissions_role FOREIGN KEY (role_id) REFERENCES public.roles(id);


--
-- Name: stock_alerts fk_stock_alerts_assigned_user; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_alerts
    ADD CONSTRAINT fk_stock_alerts_assigned_user FOREIGN KEY (assigned_user_id) REFERENCES public.users(id);


--
-- Name: stock_alerts fk_stock_alerts_equipment; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_alerts
    ADD CONSTRAINT fk_stock_alerts_equipment FOREIGN KEY (equipment_id) REFERENCES public.equipment(id);


--
-- Name: stock_alerts fk_stock_alerts_equipment_category; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_alerts
    ADD CONSTRAINT fk_stock_alerts_equipment_category FOREIGN KEY (equipment_category_id) REFERENCES public.equipment_categories(id);


--
-- Name: subscriptions fk_subscriptions_billing_plan; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT fk_subscriptions_billing_plan FOREIGN KEY (billing_plan_id) REFERENCES public.billing_plans(id);


--
-- Name: user_notification_preferences fk_user_notification_preferences_user; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_notification_preferences
    ADD CONSTRAINT fk_user_notification_preferences_user FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: user_templates fk_user_templates_role; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_templates
    ADD CONSTRAINT fk_user_templates_role FOREIGN KEY (role_id) REFERENCES public.roles(id);


--
-- Name: users fk_user_templates_users; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT fk_user_templates_users FOREIGN KEY (template_id) REFERENCES public.user_templates(id);


--
-- Name: warehouse_operations fk_warehouse_operations_equipment; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.warehouse_operations
    ADD CONSTRAINT fk_warehouse_operations_equipment FOREIGN KEY (equipment_id) REFERENCES public.equipment(id);


--
-- Name: warehouse_operations fk_warehouse_operations_installation; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.warehouse_operations
    ADD CONSTRAINT fk_warehouse_operations_installation FOREIGN KEY (installation_id) REFERENCES public.installations(id);


--
-- Name: warehouse_operations fk_warehouse_operations_user; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.warehouse_operations
    ADD CONSTRAINT fk_warehouse_operations_user FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: refresh_tokens refresh_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.local_users(id) ON DELETE CASCADE;


--
-- Name: user_accesses user_accesses_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.user_accesses
    ADD CONSTRAINT user_accesses_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_tabs user_tabs_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: com
--

ALTER TABLE ONLY public.user_tabs
    ADD CONSTRAINT user_tabs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: objects fk_contracts_objects; Type: FK CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.objects
    ADD CONSTRAINT fk_contracts_objects FOREIGN KEY (contract_id) REFERENCES tenant_default.contracts(id);


--
-- Name: contracts fk_contracts_tariff_plan; Type: FK CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.contracts
    ADD CONSTRAINT fk_contracts_tariff_plan FOREIGN KEY (tariff_plan_id) REFERENCES tenant_default.billing_plans(id);


--
-- Name: objects fk_locations_objects; Type: FK CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.objects
    ADD CONSTRAINT fk_locations_objects FOREIGN KEY (location_id) REFERENCES tenant_default.locations(id);


--
-- Name: objects fk_object_templates_objects; Type: FK CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.objects
    ADD CONSTRAINT fk_object_templates_objects FOREIGN KEY (template_id) REFERENCES tenant_default.object_templates(id);


--
-- Name: objects fk_objects_company; Type: FK CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.objects
    ADD CONSTRAINT fk_objects_company FOREIGN KEY (company_id) REFERENCES tenant_default.companies(id);


--
-- Name: role_permissions role_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.role_permissions
    ADD CONSTRAINT role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES tenant_default.permissions(id) ON DELETE CASCADE;


--
-- Name: role_permissions role_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: tenant_default; Owner: postgres
--

ALTER TABLE ONLY tenant_default.role_permissions
    ADD CONSTRAINT role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES tenant_default.roles(id) ON DELETE CASCADE;


--
-- Name: SCHEMA public; Type: ACL; Schema: -; Owner: com
--

GRANT CREATE ON SCHEMA public TO axenta_user;


--
-- PostgreSQL database dump complete
--

\unrestrict mKglDIRbBA4TZpqqCRInQiyXq7k0qVBquLvxUfQgaDzFudkT1pYxTfryxRh736n

