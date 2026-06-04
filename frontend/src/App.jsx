import { useState, useRef, useEffect, useCallback } from "react";

const SAMPLE = `Frogs are among the most remarkable vertebrates on the planet. The earliest known frog appeared roughly 190 million years ago, meaning they have shared the Earth with dinosaurs. The "horror frog" of Central Africa contracts its muscles so forcefully when threatened that it snaps its own toe bones, pushing sharp claws through the skin like a biological switchblade. Researchers have also found that certain tree frog species can navigate using Earth's magnetic field at night, orienting themselves relative to the poles when visual landmarks are unavailable. When a frog swallows food, it pulls its eyes down into the roof of its mouth to help push food down its throat.`;

const RISK = {
  verified:     { bg:"rgba(74,222,128,0.15)",  border:"#4ade80", label:"Verified",      dot:"#4ade80" },
  low:          { bg:"rgba(250,204,21,0.15)",   border:"#facc15", label:"Low Risk",      dot:"#facc15" },
  medium:       { bg:"rgba(251,146,60,0.18)",   border:"#fb923c", label:"Medium Risk",   dot:"#fb923c" },
  high:         { bg:"rgba(248,113,113,0.22)",  border:"#f87171", label:"High Risk",     dot:"#f87171" },
  unverifiable: { bg:"rgba(148,163,184,0.15)",  border:"#94a3b8", label:"Unverifiable",  dot:"#94a3b8" },
};

const API = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

function fmt$(n) { if(n<0.0001)return"<$0.0001"; return`$${n.toFixed(4)}`; }
function fmtTok(n) { return n>=1000?`${(n/1000).toFixed(1)}k`:String(n); }
function authH(t) { return{"Content-Type":"application/json",Authorization:`Bearer ${t}`}; }

function Spinner({size=12,color="#94a3b8"}) {
  return <span style={{display:"inline-block",width:size,height:size,border:`2px solid #1e2a3a`,borderTopColor:color,borderRadius:"50%",animation:"spin 0.8s linear infinite",flexShrink:0}}/>;
}

// ── Shared styles ─────────────────────────────────────────────────────────────
const S = {
  label: {fontSize:"11px",letterSpacing:"0.12em",color:"#64748b",textTransform:"uppercase",display:"block",marginBottom:"6px"},
  input: {width:"100%",background:"#0d0f14",border:"1px solid #1e2536",borderRadius:"6px",color:"#cbd5e1",fontSize:"14px",padding:"10px 12px",outline:"none",fontFamily:"inherit",boxSizing:"border-box"},
  btn:   {background:"#1d4ed8",color:"#e0eaff",border:"none",borderRadius:"6px",padding:"11px 24px",fontSize:"13px",fontFamily:"inherit",fontWeight:600,letterSpacing:"0.04em",cursor:"pointer",display:"flex",alignItems:"center",gap:"8px"},
  ghost: {background:"none",border:"1px solid #1e2a3a",color:"#64748b",fontSize:"11px",padding:"4px 10px",borderRadius:"4px",cursor:"pointer",fontFamily:"inherit"},
  card:  {background:"#0a0c10",border:"1px solid #1e2536",borderRadius:"8px",padding:"20px 24px"},
};

// ── Login ─────────────────────────────────────────────────────────────────────
function Login({onLogin}) {
  const [u,setU]=useState(""); const [p,setP]=useState("");
  const [loading,setLoading]=useState(false); const [err,setErr]=useState(null);
  async function go(e) {
    e?.preventDefault(); setLoading(true); setErr(null);
    try {
      const r=await fetch(`${API}/login`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({username:u,password:p})});
      if(!r.ok){setErr("Invalid credentials.");return;}
      onLogin((await r.json()).token);
    } catch { setErr("Could not reach server."); }
    finally { setLoading(false); }
  }
  return (
    <div style={{minHeight:"100vh",background:"#0d0f14",display:"flex",alignItems:"center",justifyContent:"center",fontFamily:"'DM Mono','Fira Mono',monospace"}}>
      <div style={{width:360,padding:"40px",...S.card}}>
        <div style={{marginBottom:"32px",textAlign:"center"}}>
          <span style={{fontSize:"11px",letterSpacing:"0.2em",color:"#4a5568",textTransform:"uppercase"}}>VERIFY</span>
          <h1 style={{margin:"8px 0 0",fontSize:"22px",fontFamily:"'DM Serif Display',Georgia,serif",color:"#f1f5f9",letterSpacing:"-0.02em"}}>Claim Inspector</h1>
        </div>
        <div style={{display:"flex",flexDirection:"column",gap:"14px"}}>
          <div><label style={S.label}>Username</label><input value={u} onChange={e=>setU(e.target.value)} autoComplete="username" style={S.input} onKeyDown={e=>e.key==="Enter"&&go()}/></div>
          <div><label style={S.label}>Password</label><input type="password" value={p} onChange={e=>setP(e.target.value)} autoComplete="current-password" style={S.input} onKeyDown={e=>e.key==="Enter"&&go()}/></div>
          {err&&<div style={{color:"#f87171",fontSize:"12px"}}>{err}</div>}
          <button onClick={go} disabled={loading||!u||!p} style={{...S.btn,justifyContent:"center",opacity:loading||!u||!p?0.5:1,marginTop:"8px"}}>
            {loading?<><Spinner/>Signing in…</>:"Sign In"}
          </button>
        </div>
      </div>
      <style>{`@keyframes spin{to{transform:rotate(360deg)}}`}</style>
    </div>
  );
}

// ── Toggle ────────────────────────────────────────────────────────────────────
function Toggle({value,onChange,label,color="#60a5fa"}) {
  return (
    <label style={{display:"flex",alignItems:"center",gap:"8px",cursor:"pointer",fontSize:"12px",color:value?color:"#475569",userSelect:"none"}}>
      <span style={{display:"inline-block",width:32,height:18,borderRadius:9,background:value?`${color}33`:"#1e2536",border:`1px solid ${value?color:"#334155"}`,position:"relative",transition:"all 0.2s"}} onClick={()=>onChange(!value)}>
        <span style={{position:"absolute",top:2,left:value?14:2,width:12,height:12,borderRadius:"50%",background:value?color:"#475569",transition:"left 0.2s"}}/>
      </span>
      {label}
    </label>
  );
}

// ── Cost bar ──────────────────────────────────────────────────────────────────
function CostBar({allTimeCost,allTimeIn,allTimeOut,sessionCost,sessionRuns}) {
  return (
    <div style={{background:"#060810",borderBottom:"1px solid #131926",padding:"5px 40px",display:"flex",alignItems:"center",gap:"20px",fontSize:"11px",color:"#334155",fontFamily:"inherit",flexWrap:"wrap"}}>
      <span style={{color:"#1e2536",letterSpacing:"0.1em",textTransform:"uppercase"}}>All-time</span>
      <span><span style={{color:"#4ade80",fontWeight:600}}>{fmt$(allTimeCost)}</span><span style={{color:"#1a2030",margin:"0 5px"}}>·</span>{fmtTok(allTimeIn)} in / {fmtTok(allTimeOut)} out</span>
      <span style={{color:"#131926",margin:"0 4px"}}>|</span>
      <span style={{color:"#1e2536",letterSpacing:"0.1em",textTransform:"uppercase"}}>This session</span>
      <span><span style={{color:"#60a5fa",fontWeight:600}}>{fmt$(sessionCost)}</span><span style={{color:"#1a2030",margin:"0 5px"}}>·</span>{sessionRuns} run{sessionRuns!==1?"s":""}</span>
      <span style={{marginLeft:"auto",color:"#0f1520"}}>Haiku 4.5 · $1/$5 per M tokens</span>
    </div>
  );
}

// ── Batch info panel ──────────────────────────────────────────────────────────
function BatchInfoPanel({estimate,onToggleOff}) {
  if(!estimate) return null;
  const syncCost = estimate.est_cost_usd;
  const batchCost = estimate.est_cost_batch_usd;
  const savings = syncCost - batchCost;
  const savingsPct = syncCost > 0 ? Math.round(savings/syncCost*100) : 50;
  return (
    <div style={{padding:"16px 20px",background:"rgba(96,165,250,0.05)",border:"1px solid rgba(96,165,250,0.2)",borderRadius:"8px",marginBottom:"14px"}}>
      <div style={{display:"flex",justifyContent:"space-between",alignItems:"flex-start",marginBottom:"12px"}}>
        <div style={{fontSize:"12px",color:"#60a5fa",fontWeight:600,letterSpacing:"0.05em"}}>BATCH MODE — ANTHROPIC BATCH API</div>
        <button onClick={onToggleOff} style={{...S.ghost,fontSize:"10px",padding:"2px 8px"}}>switch to instant</button>
      </div>
      <div style={{display:"grid",gridTemplateColumns:"1fr 1fr 1fr",gap:"12px",marginBottom:"12px"}}>
        <div style={{textAlign:"center",padding:"10px",background:"#0a0c10",borderRadius:"6px",border:"1px solid #1e2536"}}>
          <div style={{fontSize:"18px",color:"#60a5fa",fontWeight:700,marginBottom:"2px"}}>{fmt$(batchCost)}</div>
          <div style={{fontSize:"10px",color:"#475569",textTransform:"uppercase",letterSpacing:"0.1em"}}>Batch cost</div>
        </div>
        <div style={{textAlign:"center",padding:"10px",background:"#0a0c10",borderRadius:"6px",border:"1px solid #1e2536"}}>
          <div style={{fontSize:"18px",color:"#4ade80",fontWeight:700,marginBottom:"2px"}}>-{savingsPct}%</div>
          <div style={{fontSize:"10px",color:"#475569",textTransform:"uppercase",letterSpacing:"0.1em"}}>vs instant ({fmt$(syncCost)})</div>
        </div>
        <div style={{textAlign:"center",padding:"10px",background:"#0a0c10",borderRadius:"6px",border:"1px solid #1e2536"}}>
          <div style={{fontSize:"18px",color:"#fb923c",fontWeight:700,marginBottom:"2px"}}>≤24h</div>
          <div style={{fontSize:"10px",color:"#475569",textTransform:"uppercase",letterSpacing:"0.1em"}}>Processing time</div>
        </div>
      </div>
      <div style={{fontSize:"11px",color:"#475569",lineHeight:1.7}}>
        Claims are submitted to Anthropic's asynchronous Batch API. Results are polled every 8 seconds and saved to disk — you can close this tab and return later. In practice batches often complete in minutes, not hours.
      </div>
    </div>
  );
}

// ── Progress bar ──────────────────────────────────────────────────────────────
function ProgressBar({done,total,current,mode}) {
  const pct = total>0 ? Math.round(done/total*100) : 0;
  const isBatch = mode==="batch";
  return (
    <div style={{...S.card,marginBottom:"20px"}}>
      <div style={{display:"flex",justifyContent:"space-between",alignItems:"center",marginBottom:"10px"}}>
        <div style={{fontSize:"12px",color:"#94a3b8",display:"flex",alignItems:"center",gap:"8px"}}>
          <Spinner size={10} color={isBatch?"#fb923c":"#60a5fa"}/>
          <span>{isBatch?"Batch processing (Anthropic API)…":"Scoring claims in real-time…"}</span>
        </div>
        <span style={{fontSize:"11px",color:"#475569"}}>{done}/{total} · {pct}%</span>
      </div>
      <div style={{background:"#060810",borderRadius:"4px",height:"6px",overflow:"hidden",marginBottom:"8px"}}>
        <div style={{height:"100%",width:`${pct}%`,background:isBatch?"linear-gradient(90deg,#c2410c,#fb923c)":"linear-gradient(90deg,#1d4ed8,#60a5fa)",transition:"width 0.4s ease",borderRadius:"4px"}}/>
      </div>
      {current&&<div style={{fontSize:"11px",color:"#334155",fontStyle:"italic",overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap"}}>→ {current}</div>}
      {isBatch&&<div style={{marginTop:"8px",fontSize:"10px",color:"#1e2536"}}>Results saved to disk — safe to close tab and return later.</div>}
    </div>
  );
}

// ── History panel ─────────────────────────────────────────────────────────────
function fmtDateFull(s) {
  const d = new Date(s);
  return d.toLocaleDateString(undefined, { year:"numeric", month:"short", day:"numeric" })
    + " " + d.toLocaleTimeString(undefined, { hour:"2-digit", minute:"2-digit" });
}

function HistoryPanel({token,onLoad}) {
  const [runs,setRuns]       = useState(null);
  const [open,setOpen]       = useState(false);
  const [loading,setLoading] = useState(false);
  const [editing,setEditing] = useState(null);  // run id being renamed
  const [editVal,setEditVal] = useState("");

  async function fetchHistory() {
    setLoading(true);
    try {
      const r=await fetch(`${API}/history`,{headers:authH(token)});
      if(r.ok) { const d=await r.json(); setRuns(d.runs||[]); }
    } catch {}
    finally { setLoading(false); }
  }

  function toggle() {
    if(!open&&runs===null) fetchHistory();
    setOpen(o=>!o);
  }

  async function saveLabel(id) {
    await fetch(`${API}/history/${id}`,{method:"PATCH",headers:authH(token),body:JSON.stringify({label:editVal})}).catch(()=>{});
    setRuns(rs=>rs.map(r=>r.id===id?{...r,label:editVal}:r));
    setEditing(null);
  }

  function displayName(run) {
    if(run.label) return run.label;
    if(run.filename) return run.filename;
    return run.title;
  }

  const riskCounts = (claims=[]) => claims.reduce((a,c)=>({...a,[c.risk]:(a[c.risk]||0)+1}),{});

  return (
    <div style={{marginBottom:"20px"}}>
      <button onClick={toggle} style={{...S.ghost,display:"flex",alignItems:"center",gap:"6px"}}>
        <span style={{fontSize:"13px"}}>{open?"▾":"▸"}</span>
        Past fact-checks
        {runs!==null&&<span style={{color:"#334155",marginLeft:"2px"}}>({runs.length})</span>}
      </button>

      {open&&(
        <div style={{marginTop:"10px",...S.card,padding:"0",overflow:"hidden"}}>
          {loading&&<div style={{padding:"16px",fontSize:"12px",color:"#475569",display:"flex",gap:"8px",alignItems:"center"}}><Spinner size={10}/>Loading…</div>}
          {runs&&runs.length===0&&<div style={{padding:"16px",fontSize:"12px",color:"#334155"}}>No past runs yet.</div>}
          {runs&&runs.map((run,i)=>{
            const counts=riskCounts(run.claims);
            const flagged=(counts.high||0)+(counts.medium||0);
            const isEditing=editing===run.id;
            return (
              <div key={run.id} style={{padding:"12px 16px",borderBottom:i<runs.length-1?"1px solid #0d0f14":"none"}}>
                <div style={{display:"flex",alignItems:"flex-start",gap:"8px"}}>

                  {/* Main clickable area */}
                  <div style={{flex:1,minWidth:0,cursor:"pointer"}} onClick={()=>!isEditing&&onLoad(run)}>
                    {isEditing ? (
                      <div style={{display:"flex",gap:"6px",alignItems:"center"}} onClick={e=>e.stopPropagation()}>
                        <input value={editVal} onChange={e=>setEditVal(e.target.value)}
                          onKeyDown={e=>{if(e.key==="Enter")saveLabel(run.id);if(e.key==="Escape")setEditing(null);}}
                          autoFocus
                          placeholder="Enter a name for this run…"
                          style={{...S.input,padding:"4px 8px",fontSize:"12px",flex:1}}/>
                        <button onClick={()=>saveLabel(run.id)} style={{...S.ghost,fontSize:"11px",padding:"3px 8px",color:"#4ade80",borderColor:"#4ade80"}}>save</button>
                        <button onClick={()=>setEditing(null)} style={{...S.ghost,fontSize:"11px",padding:"3px 8px"}}>cancel</button>
                      </div>
                    ) : (
                      <div style={{fontSize:"13px",color:"#94a3b8",overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap",marginBottom:"4px"}}>
                        {displayName(run)}
                      </div>
                    )}

                    {/* Metadata row */}
                    <div style={{display:"flex",gap:"6px",alignItems:"center",flexWrap:"wrap",marginTop:"2px"}}>
                      <span style={{fontSize:"11px",color:"#475569"}}>{fmtDateFull(run.created_at)}</span>
                      <span style={{fontSize:"10px",color:"#1e2536"}}>·</span>
                      <span style={{fontSize:"11px",color:run.mode==="batch"?"#fb923c":"#60a5fa"}}>{run.mode}</span>
                      <span style={{fontSize:"10px",color:"#1e2536"}}>·</span>
                      <span style={{fontSize:"11px",color:"#334155"}}>{run.claims?.length||0} claims</span>
                      {flagged>0&&<><span style={{fontSize:"10px",color:"#1e2536"}}>·</span><span style={{fontSize:"11px",color:"#f87171"}}>{flagged} flagged</span></>}
                      {run.filename&&!run.label&&<><span style={{fontSize:"10px",color:"#1e2536"}}>·</span><span style={{fontSize:"11px",color:"#334155"}}>📄 {run.filename}</span></>}
                    </div>
                  </div>

                  {/* Right side: cost + rename button */}
                  <div style={{display:"flex",flexDirection:"column",alignItems:"flex-end",gap:"4px",flexShrink:0}}>
                    <span style={{fontSize:"11px",color:"#475569"}}>{fmt$(run.cost?.exact_cost_usd||0)}</span>
                    {!isEditing&&(
                      <button onClick={e=>{e.stopPropagation();setEditing(run.id);setEditVal(run.label||"");}}
                        style={{...S.ghost,fontSize:"10px",padding:"2px 6px",letterSpacing:"0.05em"}}>
                        rename
                      </button>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ── Claims results ────────────────────────────────────────────────────────────
function ClaimsView({claims,input,lastCost}) {
  const [activeIdx,setActiveIdx]=useState(null);

  function buildSegments() {
    const ordered=[...claims].map((c,i)=>({...c,idx:i,start:input.indexOf(c.text)})).filter(c=>c.start!==-1).sort((a,b)=>a.start-b.start);
    const parts=[]; let cursor=0;
    for(const c of ordered) {
      if(c.start>cursor) parts.push({type:"plain",text:input.slice(cursor,c.start)});
      parts.push({type:"claim",...c});
      cursor=c.start+c.text.length;
    }
    if(cursor<input.length) parts.push({type:"plain",text:input.slice(cursor)});
    return parts;
  }

  const segments=buildSegments();
  const counts=claims.reduce((a,c)=>({...a,[c.risk]:(a[c.risk]||0)+1}),{});

  return (
    <div style={{animation:"fadeIn 0.4s ease"}}>
      <div style={{display:"flex",gap:"8px",flexWrap:"wrap",marginBottom:"20px",alignItems:"center"}}>
        <span style={{fontSize:"11px",color:"#475569",letterSpacing:"0.1em",textTransform:"uppercase",marginRight:"4px"}}>{claims.length} claims</span>
        {Object.entries(RISK).map(([key,val])=>counts[key]?(
          <span key={key} style={{display:"inline-flex",alignItems:"center",gap:"5px",fontSize:"11px",color:val.dot,background:val.bg,border:`1px solid ${val.border}`,borderRadius:"20px",padding:"3px 10px"}}>
            <span style={{width:6,height:6,borderRadius:"50%",background:val.dot,display:"inline-block"}}/>
            {counts[key]} {val.label}
          </span>
        ):null)}
        {lastCost&&<span style={{marginLeft:"auto",fontSize:"11px",color:"#475569"}}>exact: <span style={{color:"#4ade80"}}>{fmt$(lastCost.exact_cost_usd)}</span></span>}
      </div>

      {/* Annotated text */}
      <div style={{...S.card,fontSize:"15px",lineHeight:"1.9",marginBottom:"20px"}}>
        {segments.map((p,i)=>p.type==="plain"?<span key={i}>{p.text}</span>:(
          <span key={i} onClick={()=>setActiveIdx(activeIdx===p.idx?null:p.idx)}
            style={{background:RISK[p.risk]?.bg,borderBottom:`2px solid ${RISK[p.risk]?.border}`,borderRadius:"2px",cursor:"pointer",padding:"1px 2px",outline:activeIdx===p.idx?`2px solid ${RISK[p.risk]?.border}`:"none",outlineOffset:"1px"}}>
            {p.text}
          </span>
        ))}
      </div>

      {/* Claim cards */}
      <div style={{fontSize:"11px",letterSpacing:"0.15em",color:"#475569",textTransform:"uppercase",marginBottom:"10px"}}>Claim Breakdown</div>
      <div style={{display:"flex",flexDirection:"column",gap:"8px"}}>
        {claims.map((c,i)=>{
          const col=RISK[c.risk]??RISK.unverifiable;
          const isActive=activeIdx===i;
          return (
            <div key={i} style={{border:`1px solid ${isActive?col.border:"#1e2536"}`,borderLeft:`3px solid ${col.border}`,borderRadius:"6px",padding:"12px 16px",background:isActive?col.bg:"#0a0c10",transition:"all 0.15s"}}>
              <div style={{display:"flex",alignItems:"flex-start",gap:"10px",cursor:"pointer"}} onClick={()=>setActiveIdx(isActive?null:i)}>
                <span style={{fontSize:"10px",fontWeight:700,letterSpacing:"0.1em",color:col.dot,textTransform:"uppercase",minWidth:"90px",paddingTop:"2px"}}>{col.label}</span>
                <div style={{flex:1}}>
                  <div style={{fontSize:"13px",color:"#cbd5e1",marginBottom:"4px",lineHeight:"1.5"}}>"{c.text.length>140?c.text.slice(0,140)+"…":c.text}"</div>
                  <div style={{fontSize:"12px",color:"#64748b",lineHeight:"1.6"}}>{c.explanation}</div>
                </div>
              </div>
              {c.sources?.length>0&&(
                <div style={{marginTop:"10px",paddingTop:"10px",borderTop:"1px solid #131926"}}>
                  <div style={{fontSize:"10px",color:"#1e2536",letterSpacing:"0.1em",textTransform:"uppercase",marginBottom:"6px"}}>Sources ({c.sources.length})</div>
                  <div style={{display:"flex",flexDirection:"column",gap:"6px"}}>
                    {c.sources.map((s,j)=>(
                      <div key={j} style={{display:"flex",gap:"8px",alignItems:"flex-start"}}>
                        <span style={{fontSize:"10px",color:"#1e2536",paddingTop:"3px",flexShrink:0}}>↗</span>
                        <div>
                          <a href={s.url} target="_blank" rel="noreferrer"
                            style={{fontSize:"12px",color:"#60a5fa",textDecoration:"none",display:"block",lineHeight:"1.4"}}
                            onMouseEnter={e=>e.target.style.textDecoration="underline"}
                            onMouseLeave={e=>e.target.style.textDecoration="none"}>
                            {s.title}
                          </a>
                          {s.snippet&&<div style={{fontSize:"11px",color:"#475569",lineHeight:"1.5",marginTop:"2px"}}>{s.snippet.length>180?s.snippet.slice(0,180)+"…":s.snippet}</div>}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ── Main app ──────────────────────────────────────────────────────────────────
export default function App() {
  const [token,setToken]   = useState(()=>sessionStorage.getItem("fc_token")||null);
  const [input,setInput]   = useState("");
  const [label,setLabel]   = useState("");
  const [fileName,setFileName] = useState(null);
  const [batch,setBatch]   = useState(false);
  const [claims,setClaims] = useState(null);
  const [loading,setLoading] = useState(false);
  const [error,setError]   = useState(null);
  const [lastCost,setLastCost] = useState(null);
  const [estimate,setEstimate] = useState(null);
  // Progress state
  const [progress,setProgress] = useState(null); // {done,total,current,mode}
  // Batch async job
  const [batchJob,setBatchJob] = useState(null);
  // Persistent cost totals (loaded from /history on login)
  const [allTimeCost,setAllTimeCost] = useState(0);
  const [allTimeIn,setAllTimeIn]     = useState(0);
  const [allTimeOut,setAllTimeOut]   = useState(0);
  // Session counters
  const [sessionCost,setSessionCost] = useState(0);
  const [sessionRuns,setSessionRuns] = useState(0);

  const fileRef = useRef(null);
  const estTimer = useRef(null);

  // Load all-time totals on login
  useEffect(()=>{
    if(!token) return;
    fetch(`${API}/history`,{headers:authH(token)}).then(r=>r.ok?r.json():null).then(d=>{
      if(d){ setAllTimeCost(d.total_cost_usd||0); setAllTimeIn(d.total_input_tokens||0); setAllTimeOut(d.total_output_tokens||0); }
    }).catch(()=>{});
  },[token]);

  // Debounced estimate
  const fetchEst = useCallback(async(text)=>{
    if(!text.trim()||!token){setEstimate(null);return;}
    try {
      const r=await fetch(`${API}/estimate`,{method:"POST",headers:authH(token),body:JSON.stringify({text})});
      if(r.ok) setEstimate(await r.json());
    } catch {}
  },[token]);

  useEffect(()=>{
    clearTimeout(estTimer.current);
    if(!input.trim()){setEstimate(null);return;}
    estTimer.current=setTimeout(()=>fetchEst(input),600);
    return()=>clearTimeout(estTimer.current);
  },[input,fetchEst]);

  function handleLogin(t){sessionStorage.setItem("fc_token",t);setToken(t);}
  async function handleLogout(){
    await fetch(`${API}/logout`,{method:"POST",headers:authH(token)}).catch(()=>{});
    sessionStorage.removeItem("fc_token"); setToken(null);
    setClaims(null); setInput(""); setFileName(null); setLabel(""); setEstimate(null); setLastCost(null);
  }

  function handleFileChange(e){
    const f=e.target.files?.[0]; if(!f) return;
    setFileName(f.name);
    if(!label) setLabel(f.name.replace(/\.[^.]+$/,"")); // pre-fill label with filename (sans extension)
    if(f.size>15000) setBatch(true);
    const reader=new FileReader();
    reader.onload=ev=>{setInput(ev.target.result);setClaims(null);};
    reader.readAsText(f);
  }

  function onCostRecorded(cost){
    setLastCost(cost);
    setSessionCost(s=>s+cost.exact_cost_usd);
    setSessionRuns(s=>s+1);
    setAllTimeCost(s=>s+cost.exact_cost_usd);
    setAllTimeIn(s=>s+cost.usage.input_tokens);
    setAllTimeOut(s=>s+cost.usage.output_tokens);
  }

  async function analyze(){
    if(!input.trim()) return;
    setLoading(true); setError(null); setClaims(null); setLastCost(null); setProgress(null);

    if(batch){
      // Async batch — submit and poll
      try {
        const r=await fetch(`${API}/analyze`,{method:"POST",headers:authH(token),body:JSON.stringify({text:input,batch:true,label,filename:fileName||""})});
        if(r.status===401){sessionStorage.removeItem("fc_token");setToken(null);return;}
        if(r.status===402){setError(await r.text());return;}
        if(!r.ok) throw new Error(`Server error: ${r.status}`);
        const d=await r.json();
        if(d.batch_id){
          setBatchJob({id:d.batch_id,progress:0,done:0,total:0});
          setProgress({done:0,total:0,current:"",mode:"batch"});
          pollBatch(d.batch_id);
          return;
        }
      } catch(e){setError(e.message);}
      finally{if(!batchJob)setLoading(false);}
    } else {
      // Sync — use SSE stream endpoint
      try {
        const resp = await fetch(`${API}/analyze/stream`,{
          method:"POST", headers:authH(token), body:JSON.stringify({text:input,batch:false,label,filename:fileName||""})
        });
        if(resp.status===401){sessionStorage.removeItem("fc_token");setToken(null);setLoading(false);return;}
        if(resp.status===402){setError(await resp.text());setLoading(false);return;}
        if(!resp.ok) throw new Error(`Server error: ${resp.status}`);

        const reader=resp.body.getReader();
        const decoder=new TextDecoder();
        let buf="";

        while(true){
          const {done,value}=await reader.read();
          if(done) break;
          buf+=decoder.decode(value,{stream:true});
          // Parse SSE lines
          const lines=buf.split("\n");
          buf=lines.pop(); // last incomplete line stays in buffer
          let eventType=""; let dataLine="";
          for(const line of lines){
            if(line.startsWith("event:")) eventType=line.slice(6).trim();
            else if(line.startsWith("data:")) dataLine=line.slice(5).trim();
            else if(line===""&&eventType&&dataLine){
              try {
                const payload=JSON.parse(dataLine);
                if(eventType==="extracted") setProgress({done:0,total:payload.total,current:"",mode:"sync"});
                else if(eventType==="progress") setProgress(p=>({...p,done:payload.done,total:payload.total,current:payload.current,mode:"sync"}));
                else if(eventType==="done"){
                  setClaims(payload.claims||[]);
                  if(payload.cost) onCostRecorded(payload.cost);
                  setProgress(null); setLoading(false);
                }
                else if(eventType==="error"){ setError(payload.message); setProgress(null); setLoading(false); }
              } catch {}
              eventType=""; dataLine="";
            }
          }
        }
      } catch(e){ setError(e.message); setProgress(null); setLoading(false); }
    }
  }

  async function pollBatch(batchId){
    const iv=setInterval(async()=>{
      try {
        const r=await fetch(`${API}/batch/${batchId}`,{headers:authH(token)});
        if(!r.ok){clearInterval(iv);setError("Batch status check failed.");setLoading(false);setBatchJob(null);setProgress(null);return;}
        const d=await r.json();
        setBatchJob({id:batchId,progress:d.progress||0,done:d.succeeded||0,total:d.total||0});
        setProgress({done:d.succeeded||0,total:d.total||0,current:"",mode:"batch"});
        if(d.status==="done"){
          clearInterval(iv);
          setClaims(d.claims||[]);
          if(d.cost) onCostRecorded(d.cost);
          setLoading(false); setBatchJob(null); setProgress(null);
        } else if(d.status==="failed"){
          clearInterval(iv); setError(d.error||"Batch failed."); setLoading(false); setBatchJob(null); setProgress(null);
        }
      } catch(e){clearInterval(iv);setError(e.message);setLoading(false);setBatchJob(null);setProgress(null);}
    },8000);
  }

  function loadHistoryRun(run){
    setInput(run.title);
    setLabel(run.label||"");
    setFileName(run.filename||null);
    setClaims(run.claims);
    setLastCost(run.cost);
    setProgress(null); setError(null);
  }

  if(!token) return <Login onLogin={handleLogin}/>;

  const isLarge=input.length>3000;
  const estPill = estimate&&(
    <span style={{fontSize:"11px",color:"#94a3b8",background:"#0f1117",border:"1px solid #1e2536",borderRadius:"20px",padding:"3px 10px",display:"inline-flex",alignItems:"center",gap:"6px"}}>
      <span style={{width:6,height:6,borderRadius:"50%",background:"#60a5fa",display:"inline-block"}}/>
      ~{estimate.estimated_claims} claims · <span style={{color:"#60a5fa",fontWeight:600}}>{fmt$(batch?estimate.est_cost_batch_usd:estimate.est_cost_usd)}</span>
    </span>
  );

  return (
    <div style={{minHeight:"100vh",background:"#0d0f14",fontFamily:"'DM Mono','Fira Mono','Courier New',monospace",color:"#e2e8f0"}}>
      <style>{`
        @keyframes spin{to{transform:rotate(360deg)}}
        @keyframes fadeIn{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
        input[type=file]{display:none}
      `}</style>

      <CostBar allTimeCost={allTimeCost} allTimeIn={allTimeIn} allTimeOut={allTimeOut} sessionCost={sessionCost} sessionRuns={sessionRuns}/>

      {/* Header */}
      <div style={{borderBottom:"1px solid #1e2536",padding:"20px 40px",display:"flex",alignItems:"center",gap:"14px"}}>
        <span style={{fontSize:"11px",letterSpacing:"0.2em",color:"#4a5568",textTransform:"uppercase"}}>VERIFY</span>
        <h1 style={{margin:0,fontSize:"20px",fontWeight:700,fontFamily:"'DM Serif Display',Georgia,serif",color:"#f1f5f9",letterSpacing:"-0.02em"}}>Claim Inspector</h1>
        <span style={{fontSize:"11px",color:"#4a5568",marginLeft:"auto"}}>Wikipedia · Semantic Scholar · arXiv · PubMed</span>
        <button onClick={handleLogout} style={{...S.ghost}}>sign out</button>
      </div>

      <div style={{maxWidth:"900px",margin:"0 auto",padding:"32px 40px"}}>

        <HistoryPanel token={token} onLoad={loadHistoryRun}/>

        {/* Input */}
        <div style={{marginBottom:"14px"}}>
          <div style={{display:"flex",justifyContent:"space-between",alignItems:"center",marginBottom:"10px"}}>
            <label style={S.label}>Input Text</label>
            <div style={{display:"flex",gap:"8px",alignItems:"center"}}>
              {estPill}
              <button onClick={()=>{setInput(SAMPLE);setFileName(null);setLabel("");setClaims(null);setBatch(false);}} style={S.ghost}>load sample</button>
              <button onClick={()=>fileRef.current?.click()} style={S.ghost}>upload file</button>
              <input ref={fileRef} type="file" accept=".txt,.md,.html,.htm,.docx,.pdf" onChange={handleFileChange}/>
            </div>
          </div>
          <div onDragOver={e=>e.preventDefault()}
            onDrop={e=>{e.preventDefault();const f=e.dataTransfer.files?.[0];if(f){const dt=new DataTransfer();dt.items.add(f);fileRef.current.files=dt.files;handleFileChange({target:{files:dt.files}});}}}
            style={{border:"1px solid #1e2536",borderRadius:"8px",background:"#0a0c10"}}>
            {fileName&&<div style={{padding:"8px 18px 0",fontSize:"11px",color:"#4ade80",display:"flex",alignItems:"center",gap:"6px"}}>
              📄 {fileName}
              <button onClick={()=>{setFileName(null);setInput("");setClaims(null);if(fileRef.current)fileRef.current.value="";}} style={{background:"none",border:"none",color:"#64748b",cursor:"pointer",fontSize:"14px",padding:"0 4px"}}>×</button>
            </div>}
            <textarea value={input} onChange={e=>{setInput(e.target.value);setClaims(null);setFileName(null);}} placeholder="Paste text here, or drag & drop a file (.txt, .md, .html, .docx, .pdf)…" rows={8}
              style={{width:"100%",background:"transparent",border:"none",color:"#cbd5e1",fontSize:"14px",lineHeight:"1.7",padding:"16px 18px",resize:"vertical",outline:"none",fontFamily:"inherit",boxSizing:"border-box"}}/>
          </div>
          <div style={{fontSize:"11px",color:"#1e2536",marginTop:"5px"}}>
            Supported: <span style={{color:"#334155"}}>.txt .md .html .docx .pdf</span>
            <span style={{color:"#131926",margin:"0 6px"}}>·</span>
            <span style={{color:"#1a2030"}}>Scanned PDFs not supported</span>
          </div>
        </div>

        {/* Run label */}
        <div style={{marginBottom:"14px"}}>
          <input value={label} onChange={e=>setLabel(e.target.value)} placeholder="Name this fact-check (optional — shown in history)"
            style={{...S.input,fontSize:"13px",color:label?"#cbd5e1":"#334155"}}/>

        {/* Batch info panel or simple toggle */}
        {batch ? (
          <BatchInfoPanel estimate={estimate} onToggleOff={()=>setBatch(false)}/>
        ) : (
          <div style={{marginBottom:"14px"}}>
            {isLarge ? (
              <div style={{padding:"12px 16px",background:"rgba(251,146,60,0.06)",border:"1px solid rgba(251,146,60,0.2)",borderRadius:"8px",display:"flex",alignItems:"center",gap:"12px"}}>
                <span style={{fontSize:"14px"}}>⚠</span>
                <div style={{flex:1,fontSize:"12px",color:"#64748b",lineHeight:1.6}}>
                  Large document — <span style={{color:"#fb923c"}}>batch mode saves {estimate?`~${fmt$(estimate.est_cost_usd-estimate.est_cost_batch_usd)}`:"~50%"}</span> but takes up to 24h via Anthropic's async API.
                </div>
                <Toggle value={batch} onChange={setBatch} label="Use batch" color="#fb923c"/>
              </div>
            ) : (
              <Toggle value={batch} onChange={setBatch} label={`Batch mode — Anthropic async API · 50% cheaper · up to 24h${estimate?` · saves ${fmt$(estimate.est_cost_usd-estimate.est_cost_batch_usd)}`:""}`} color="#60a5fa"/>
            )}
          </div>
        )}

        {/* Analyze button */}
        <div style={{display:"flex",alignItems:"center",gap:"14px",marginBottom:"28px"}}>
          <button onClick={analyze} disabled={loading||!input.trim()}
            style={{...S.btn,background:loading?"#1e2536":"#1d4ed8",color:loading?"#475569":"#e0eaff",cursor:loading||!input.trim()?"not-allowed":"pointer",opacity:!input.trim()?0.5:1}}>
            {loading?<><Spinner/>{batch?"Submitting batch…":"Analyzing…"}</>:"Analyze Claims"}
          </button>
          {lastCost&&!loading&&(
            <div style={{fontSize:"11px",color:"#475569",display:"flex",gap:"8px",alignItems:"center"}}>
              Last run: <span style={{color:"#4ade80",fontWeight:600}}>{fmt$(lastCost.exact_cost_usd)}</span>
              <span style={{color:"#1e2536"}}>·</span>
              {fmtTok(lastCost.usage.input_tokens)} in / {fmtTok(lastCost.usage.output_tokens)} out
            </div>
          )}
        </div>

        {error&&<div style={{color:"#f87171",fontSize:"13px",marginBottom:"20px",padding:"12px 16px",background:"rgba(248,113,113,0.08)",borderRadius:"6px",border:"1px solid rgba(248,113,113,0.2)"}}>{error}</div>}

        {progress&&<ProgressBar done={progress.done} total={progress.total} current={progress.current} mode={progress.mode}/>}

        {claims&&<ClaimsView claims={claims} input={input} lastCost={lastCost}/>}
      </div>
      </div>
    </div>
  );
}
