CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS pod_metrics (
    time          TIMESTAMPTZ      NOT NULL,
    namespace     TEXT             NOT NULL,
    pod           TEXT             NOT NULL,
    cpu           DOUBLE PRECISION NOT NULL,
    memory        DOUBLE PRECISION NOT NULL,
    status        TEXT             NOT NULL,
    restart_count INTEGER          NOT NULL
);

SELECT create_hypertable('pod_metrics', 'time', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS pod_metrics_namespace_pod_time_idx
    ON pod_metrics (namespace, pod, time DESC);

CREATE TABLE IF NOT EXISTS anomalies (
    time      TIMESTAMPTZ      NOT NULL,
    namespace TEXT             NOT NULL,
    pod       TEXT             NOT NULL,
    metric    TEXT             NOT NULL,
    value     DOUBLE PRECISION NOT NULL,
    baseline  DOUBLE PRECISION NOT NULL,
    severity  TEXT             NOT NULL
);

CREATE INDEX IF NOT EXISTS anomalies_namespace_pod_time_idx
    ON anomalies (namespace, pod, time DESC);
