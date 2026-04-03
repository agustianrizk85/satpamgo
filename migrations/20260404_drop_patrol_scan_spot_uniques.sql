do $$
declare
  constraint_row record;
  index_row record;
begin
  -- Model terbaru memperlakukan patrol_scans sebagai master scan. Spot yang sama
  -- harus bisa tersimpan berulang untuk distribusi ke Ronde 1..N dan sisanya ke
  -- bucket Tanpa Ronde. Karena itu, unique apa pun yang melibatkan spot_id di
  -- patrol_scans harus dibuang, termasuk yang ikut membawa patrol_run_id.

  for constraint_row in
    select con.conname
    from pg_constraint con
    where con.conrelid = 'patrol_scans'::regclass
      and con.contype = 'u'
      and exists (
        select 1
        from unnest(con.conkey) as key_col(attnum)
        join pg_attribute attr
          on attr.attrelid = con.conrelid
         and attr.attnum = key_col.attnum
        where attr.attname = 'spot_id'
      )
  loop
    execute format('alter table patrol_scans drop constraint if exists %I', constraint_row.conname);
  end loop;

  for index_row in
    select idx_class.relname as index_name
    from pg_index idx
    join pg_class table_class
      on table_class.oid = idx.indrelid
    join pg_class idx_class
      on idx_class.oid = idx.indexrelid
    where table_class.oid = 'patrol_scans'::regclass
      and idx.indisunique = true
      and idx.indisprimary = false
      and exists (
        select 1
        from unnest(idx.indkey) as key_col(attnum)
        join pg_attribute attr
          on attr.attrelid = idx.indrelid
         and attr.attnum = key_col.attnum
        where attr.attname = 'spot_id'
      )
  loop
    execute format('drop index if exists %I', index_row.index_name);
  end loop;
end $$;
