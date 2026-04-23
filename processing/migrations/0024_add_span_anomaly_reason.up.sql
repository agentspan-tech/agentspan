-- Add anomaly columns to spans for LLM-detected content anomalies.
ALTER TABLE spans ADD COLUMN anomaly_reason TEXT;
ALTER TABLE spans ADD COLUMN anomaly_category TEXT;
