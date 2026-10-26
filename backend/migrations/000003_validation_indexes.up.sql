CREATE INDEX experiment_trials_strategy_version_idx
    ON experiment_trials(strategy_name, strategy_version);

CREATE INDEX holdout_access_strategy_version_idx
    ON holdout_access_log(strategy_name, strategy_version, accessed_at);

CREATE INDEX universe_membership_asof_lookup_idx
    ON market_universe_memberships(universe_id, valid_from, valid_to, stock_id);
