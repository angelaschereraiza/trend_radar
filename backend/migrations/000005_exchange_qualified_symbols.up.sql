ALTER TABLE symbol_history
    ADD COLUMN exchange TEXT NOT NULL DEFAULT '';

UPDATE symbol_history AS history
SET exchange = stocks.exchange
FROM stocks
WHERE stocks.id = history.stock_id;

DROP INDEX symbol_history_current_symbol_idx;

CREATE UNIQUE INDEX symbol_history_current_symbol_idx
    ON symbol_history(exchange, symbol) WHERE valid_to IS NULL;
