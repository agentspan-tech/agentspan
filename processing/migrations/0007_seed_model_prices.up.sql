CREATE TABLE model_prices (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_type           TEXT NOT NULL,
    model                   TEXT NOT NULL,
    input_price_per_token   NUMERIC(20, 10) NOT NULL,
    output_price_per_token  NUMERIC(20, 10) NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider_type, model)
);

CREATE INDEX idx_model_prices_provider_model ON model_prices(provider_type, model);

-- Seed: OpenAI prices (USD per token, as of 2025)
INSERT INTO model_prices (provider_type, model, input_price_per_token, output_price_per_token) VALUES
    ('openai', 'gpt-4o',                    0.0000025,  0.000010),
    ('openai', 'gpt-4o-mini',               0.00000015, 0.0000006),
    ('openai', 'gpt-4-turbo',               0.000010,   0.000030),
    ('openai', 'gpt-4',                     0.000030,   0.000060),
    ('openai', 'gpt-3.5-turbo',             0.0000005,  0.0000015),
    ('openai', 'o1',                        0.000015,   0.000060),
    ('openai', 'o1-mini',                   0.000003,   0.000012),
    ('openai', 'o3-mini',                   0.0000011,  0.0000044);

-- Seed: Anthropic prices
INSERT INTO model_prices (provider_type, model, input_price_per_token, output_price_per_token) VALUES
    ('anthropic', 'claude-opus-4-5',        0.000015,   0.000075),
    ('anthropic', 'claude-sonnet-4-5',      0.000003,   0.000015),
    ('anthropic', 'claude-haiku-3-5',       0.0000008,  0.000004),
    ('anthropic', 'claude-3-opus-20240229', 0.000015,   0.000075),
    ('anthropic', 'claude-3-5-sonnet-20241022', 0.000003, 0.000015),
    ('anthropic', 'claude-3-haiku-20240307', 0.00000025, 0.00000125);

-- Seed: DeepSeek prices
INSERT INTO model_prices (provider_type, model, input_price_per_token, output_price_per_token) VALUES
    ('deepseek', 'deepseek-chat',           0.00000027, 0.0000011),
    ('deepseek', 'deepseek-reasoner',       0.00000055, 0.0000022);

-- Seed: Mistral prices
INSERT INTO model_prices (provider_type, model, input_price_per_token, output_price_per_token) VALUES
    ('mistral', 'mistral-large-latest',     0.000002,   0.000006),
    ('mistral', 'mistral-small-latest',     0.0000002,  0.0000006),
    ('mistral', 'codestral-latest',         0.000001,   0.000003),
    ('mistral', 'mistral-nemo',             0.00000015, 0.00000015);

-- Seed: Groq prices
INSERT INTO model_prices (provider_type, model, input_price_per_token, output_price_per_token) VALUES
    ('groq', 'llama-3.3-70b-versatile',    0.00000059, 0.00000079),
    ('groq', 'llama-3.1-8b-instant',       0.00000005, 0.00000008),
    ('groq', 'mixtral-8x7b-32768',         0.00000024, 0.00000024),
    ('groq', 'gemma2-9b-it',               0.0000002,  0.0000002);

-- Seed: Gemini prices
INSERT INTO model_prices (provider_type, model, input_price_per_token, output_price_per_token) VALUES
    ('gemini', 'gemini-2.0-flash',          0.0000001,  0.0000004),
    ('gemini', 'gemini-1.5-pro',            0.00000125, 0.000005),
    ('gemini', 'gemini-1.5-flash',          0.000000075, 0.0000003),
    ('gemini', 'gemini-1.5-flash-8b',       0.0000000375, 0.00000015);
