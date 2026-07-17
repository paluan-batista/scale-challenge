CREATE INDEX weighings_weighed_at_idx ON weighings (weighed_at);
CREATE INDEX weighings_branch_id_idx ON weighings (branch_id);
CREATE INDEX weighings_grain_type_id_idx ON weighings (grain_type_id);
CREATE INDEX transport_transactions_weighed_status_idx ON transport_transactions (id) WHERE status = 'WEIGHED';
