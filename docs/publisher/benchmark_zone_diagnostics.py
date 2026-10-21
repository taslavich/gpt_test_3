#!/usr/bin/env python3
"""Disposable ClickHouse only. Run via run_ch.sh. No production credentials."""
import json,pathlib,re,time,uuid,threading,urllib.request,urllib.parse,statistics,os
project=pathlib.Path(__file__).resolve().parents[2]
import argparse,math
p=argparse.ArgumentParser(description='DISPOSABLE LOCAL ClickHouse only: HTTP 18123, query_log enabled; never production')
p.add_argument('--disposable-local',action='store_true',required=True)
p.add_argument('--rows',type=int,nargs='+',default=[12000000,36000000])
p.add_argument('--memory-mib',type=int,choices=[512,1024],default=1024)
p.add_argument('--output',type=pathlib.Path,default=pathlib.Path('zone-benchmark-results'))
a=p.parse_args()
if any(n<1000 or n>36000000 for n in a.rows):p.error('bounded benchmark: rows=1000..36000000')
root=a.output.resolve();(root/'logs').mkdir(parents=True,exist_ok=True)
database='zone_bench_'+uuid.uuid4().hex
url='http://127.0.0.1:18123/'
def sql(q,qid=None):
 q=q.replace('bench.',database+'.').replace("database='bench'","database='"+database+"'").replace('CREATE DATABASE bench','CREATE DATABASE '+database)
 try:
  with urllib.request.urlopen(urllib.request.Request(url+('?'+urllib.parse.urlencode({'query_id':qid}) if qid else ''),q.encode()),timeout=180) as r:return r.read()
 except urllib.error.HTTPError as e:raise RuntimeError(e.read().decode()[:2500]) from e
def get(q):return json.loads(sql(q+' FORMAT JSON'))['data']
sql('CREATE DATABASE bench')
src=(project/'internal/services/clickhouse-loader/createDb.go').read_text()
ddl=re.search(r'CREATE TABLE IF NOT EXISTS \{db\}\.ortb\s*\([\s\S]*?SETTINGS index_granularity = 8192;',src).group().replace('{db}','bench')
sql(ddl);sql('SYSTEM STOP MERGES bench.ortb')
sql('CREATE TABLE bench.probe(g UInt64,v UInt64) ENGINE=MergeTree ORDER BY g')
sql('INSERT INTO bench.probe SELECT number%100,number FROM numbers(1000000)')
source=(project/'internal/services/clickhouse-loader/publisher_zone_diagnostics.go').read_text()
query=re.search(r'q := `([\s\S]+?)`',source).group(1).replace('{db}','bench').replace('{minutes}','45')
query=query.replace('max_memory_usage=1073741824','max_memory_usage='+str(a.memory_mib*1048576))
query=query.replace('SETTINGS max_threads=2,','SETTINGS max_execution_time=120,max_threads=2,')+' FORMAT Null'
(root/'logs/benchmark-select.sql').write_text(query)
plan=sql('EXPLAIN indexes=1 '+query.removesuffix(' FORMAT Null')).decode();(root/'logs/benchmark-explain.txt').write_text(plan)
base=get("SELECT formatDateTime(now64(3,'UTC'),'%Y-%m-%d %H:%i:%S','UTC') AS t")[0]['t']
probe='SELECT sum(v) FROM bench.probe WHERE g<20 SETTINGS max_threads=1 FORMAT Null'
def latency():
 t=time.perf_counter();sql(probe);return (time.perf_counter()-t)*1000
baseline=[latency() for _ in range(20)]
def pct(v,p):return sorted(v)[max(0,min(len(v)-1,math.ceil(len(v)*p)-1))] if v else None
result={'source':'actual proj234 ORTB DDL; no financial/discovery MV on benchmark table','publisher_share':0.10,'logical_id':'UUID-shaped; deterministic 1% total Kafka replay (10% of Publisher rows)','settings':{'max_threads':2,'max_memory_usage':a.memory_mib*1048576,'spill_threshold':134217728,'max_execution_time':120},'critical_probe':{'query':probe,'baseline_p50_ms':statistics.median(baseline),'baseline_p95_ms':pct(baseline,.95)},'runs':[]}
inserted=0
for target in sorted(set(a.rows)):
 while inserted<target:
  n=min(250000,target-inserted)
  # Five percent deliberately expired parts exercise the created_at index/PREWHERE.
  q=f"""INSERT INTO bench.ortb(uuid,logical_event_id,event_time,created_at,code,format,pub_user_id,ad_code_id,zone_id,site_id,site_domain,geo,device,os,browser)
 SELECT id,toString(id),t,t,multiIf(k%4=0,200,k%4=1,204,k%4=2,403,503),'POP',if(k%10=0,'publisher-01',''),'code',concat('zone-',toString(intDiv(k,10)%1000)),'site','example.test','US','desktop','Windows','Chrome'
 FROM (SELECT k,reinterpretAsUUID(MD5(toString(k))) AS id,toDateTime64('{base}',3,'UTC')-toIntervalSecond(if(k%20=19,5400,k%3600+1)) AS t FROM (SELECT if(number%100=10,toUInt64(number-10),number) AS k FROM numbers({inserted},{n}))) SETTINGS max_block_size=65536,min_insert_block_size_rows=65536,min_insert_block_size_bytes=1048576,max_insert_block_size=65536,max_threads=2,max_memory_usage=1073741824"""
  sql(q);inserted+=n
 sql('EXPLAIN actions=1,indexes=1 '+query.removesuffix(' FORMAT Null'))
 (root/'logs/benchmark-explain.txt').write_text(sql('EXPLAIN actions=1,indexes=1 '+query.removesuffix(' FORMAT Null')).decode())
 for repeat in range(2):
  qid='tb_zone_bench_'+uuid.uuid4().hex
  running=threading.Event();running.set();latencies=[];peak_temp=[0]
  def observe():
   temp=pathlib.Path(os.environ.get('ZONE_BENCH_TEMP',str(root/'unavailable-temp')))
   while running.is_set():
    try:
     peak_temp[0]=max(peak_temp[0],sum(f.stat().st_size for f in temp.rglob('*') if f.is_file()))
     latencies.append(latency())
    except (OSError,urllib.error.URLError):pass
    time.sleep(.05)
  thread=threading.Thread(target=observe,daemon=True);thread.start()
  start=time.perf_counter();error=None
  try:sql(query,qid)
  except Exception as e:error=str(e)
  elapsed=time.perf_counter()-start;running.clear();thread.join()
  logs=[]
  for attempt in range(20):
   sql('SYSTEM FLUSH LOGS')
   logs=get("SELECT type,query_duration_ms,read_rows,read_bytes,memory_usage,ProfileEvents,exception FROM system.query_log WHERE query_id='"+qid+"' AND type IN ('QueryFinish','ExceptionWhileProcessing','ExceptionBeforeStart') ORDER BY event_time_microseconds DESC LIMIT 1")
   if logs:break
   time.sleep(.1)
  log=logs[0] if logs else {};events=log.pop('ProfileEvents',{})
  selected={k:v for k,v in events.items() if 'External' in k or k in ['UserTimeMicroseconds','SystemTimeMicroseconds','OSReadBytes','OSWriteBytes','DiskWriteElapsedMicroseconds','SelectedMarks','SelectedRows','RowsReadByPrewhereReaders','RowsReadByMainReader']}
  result['runs'].append({'source_rows':target,'repeat':repeat,'elapsed_seconds':elapsed,'max_observed_temp_bytes':peak_temp[0] if 'ZONE_BENCH_TEMP' in os.environ else None,'probe_p50_ms':statistics.median(latencies) if latencies else None,'probe_p95_ms':pct(latencies,.95),'error':error,**log,'events':selected})
  (root/'logs/benchmark-results.json').write_text(json.dumps(result,indent=2))
  print(json.dumps(result['runs'][-1]),flush=True)
 if any(r['error'] for r in result['runs']):break
result['source_rows_actual']=get('SELECT count() AS n FROM bench.ortb')[0]['n']
result['parts']=get("SELECT sum(bytes_on_disk) AS disk_bytes,sum(rows) AS rows FROM system.parts WHERE database='bench' AND table='ortb' AND active")[0]
(root/'logs/benchmark-results.json').write_text(json.dumps(result,indent=2))
