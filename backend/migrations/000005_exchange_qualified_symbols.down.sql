DROP INDEX symbol_history_current_symbol_idx;

ALTER TABLE symbol_history
    DROP COLUMN exchange;

CREATE UNIQUE INDEX symbol_history_current_symbol_idx
    ON symbol_history(symbol) WHERE valid_to IS NULL;
