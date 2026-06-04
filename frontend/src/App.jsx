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

// Readable contrast on dark backgrounds (avoid #1e2536 / #0f1520 for text)
const C = {
  text:      "#e8edf4",
  secondary: "#b8c5d6",
  muted:     "#94a3b8",
  faint:     "#7c8da3",
  label:     "#a8b8cc",
  border:    "#3d4f66",
  borderDim: "#2a3548",
  surface:   "#141820",
  surfaceHi: "#1a2030",
  bar:       "#10141c",
};

function fmt$(n) { if(n<0.0001)return"<$0.0001"; return`$${n.toFixed(4)}`; }
function fmtTok(n) { return n>=1000?`${(n/1000).toFixed(1)}k`:String(n); }
function authH(t) { return{"Content-Type":"application/json",Authorization:`Bearer ${t}`}; }

function haikuFromEstimate(est) {
  if(!est) return 0;
  if(est.anthropic_cost_usd!=null) return est.anthropic_cost_usd;
  return (est.est_cost_usd||0)-(est.est_aux_cost_usd||0);
}
function haikuBatchFromEstimate(est) {
  if(!est) return 0;
  return (est.est_cost_batch_usd||0)-(est.est_aux_cost_usd||0);
}
function auxFromCost(cost) {
  return (cost?.aux_costs||[]).reduce((s,l)=>s+(l.amount_usd||0),0);
}
function haikuFromCost(cost) {
  if(!cost) return 0;
  return cost.anthropic_cost_usd ?? (cost.exact_cost_usd - auxFromCost(cost));
}
function auxCostTitle(lines=[]) {
  return lines.map(l=>`${l.label}: ${fmt$(l.amount_usd||0)}${l.note?` (${l.note})`:""}`).join("\n");
}

function CostSummary({cost,label="exact",style={}}) {
  if(!cost) return null;
  const total=cost.exact_cost_usd||0;
  const haiku=haikuFromCost(cost);
  const aux=auxFromCost(cost);
  const lines=cost.aux_costs||[];
  const showSplit=lines.length>0||cost.anthropic_cost_usd!=null;
  const auxNote=aux<=0.00001&&lines.some(l=>l.note)?" · APIs $0 (free tier)":aux>0.00001?` · APIs ${fmt$(aux)}`:"";
  return (
    <span style={style} title={lines.length?auxCostTitle(lines):undefined}>
      {label}: <span style={{color:"#4ade80",fontWeight:700}}>{fmt$(total)}</span>
      {showSplit&&<span style={{fontSize:"11px",color:C.muted,marginLeft:"6px"}}>(Haiku {fmt$(haiku)}{auxNote})</span>}
    </span>
  );
}

function Spinner({size=12,color="#94a3b8"}) {
  return <span style={{display:"inline-block",width:size,height:size,border:`2px solid #1e2a3a`,borderTopColor:color,borderRadius:"50%",animation:"spin 0.8s linear infinite",flexShrink:0}}/>;
}

// ── Shared styles ─────────────────────────────────────────────────────────────
const S = {
  label: {fontSize:"12px",letterSpacing:"0.1em",color:C.label,fontWeight:600,textTransform:"uppercase",display:"block",marginBottom:"6px"},
  input: {width:"100%",background:"#0d0f14",border:`1px solid ${C.border}`,borderRadius:"6px",color:C.text,fontSize:"14px",padding:"10px 12px",outline:"none",fontFamily:"inherit",boxSizing:"border-box"},
  btn:   {background:"#1d4ed8",color:"#e0eaff",border:"none",borderRadius:"6px",padding:"11px 24px",fontSize:"13px",fontFamily:"inherit",fontWeight:600,letterSpacing:"0.04em",cursor:"pointer",display:"flex",alignItems:"center",gap:"8px"},
  ghost: {background:C.surfaceHi,border:`1px solid ${C.border}`,color:C.secondary,fontSize:"12px",padding:"5px 12px",borderRadius:"4px",cursor:"pointer",fontFamily:"inherit"},
  card:  {background:"#0a0c10",border:`1px solid ${C.borderDim}`,borderRadius:"8px",padding:"20px 24px"},
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
          <span style={{fontSize:"11px",letterSpacing:"0.2em",color:C.muted,textTransform:"uppercase"}}>VERIFY</span>
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
    <label style={{display:"flex",alignItems:"center",gap:"8px",cursor:"pointer",fontSize:"13px",color:value?color:C.secondary,userSelect:"none",fontWeight:value?600:400}}>
      <span style={{display:"inline-block",width:32,height:18,borderRadius:9,background:value?`${color}44`:"#252a35",border:`1px solid ${value?color:C.border}`,position:"relative",transition:"all 0.2s",flexShrink:0}} onClick={()=>onChange(!value)}>
        <span style={{position:"absolute",top:2,left:value?14:2,width:12,height:12,borderRadius:"50%",background:value?color:C.muted,transition:"left 0.2s"}}/>
      </span>
      {label}
    </label>
  );
}

function fmtUsage(n,unit){
  if(unit==="usd") return fmt$(n);
  if(unit==="tokens") return fmtTok(Math.round(n));
  return Math.round(n).toLocaleString();
}

// ── Cost + API usage bars ─────────────────────────────────────────────────────
function CostBar({allTimeCost,allTimeIn,allTimeOut,sessionCost,sessionRuns}) {
  const block = (title, children) => (
    <div style={{display:"flex",alignItems:"baseline",gap:"10px",flexWrap:"wrap"}}>
      <span style={{color:C.label,letterSpacing:"0.08em",textTransform:"uppercase",fontSize:"11px",fontWeight:700,minWidth:"72px"}}>{title}</span>
      {children}
    </div>
  );
  return (
    <div style={{background:C.bar,borderBottom:`1px solid ${C.border}`,padding:"12px 40px",display:"flex",alignItems:"center",gap:"28px",fontSize:"13px",color:C.secondary,flexWrap:"wrap"}}>
      {block("All-time", <>
        <span style={{color:"#4ade80",fontWeight:700,fontSize:"15px"}}>{fmt$(allTimeCost)}</span>
        <span style={{color:C.faint}}>·</span>
        <span style={{color:C.text,fontWeight:500}}>{fmtTok(allTimeIn)} in / {fmtTok(allTimeOut)} out</span>
      </>)}
      <span style={{color:C.border,fontSize:"18px",lineHeight:1}}>|</span>
      {block("This session", <>
        <span style={{color:"#60a5fa",fontWeight:700,fontSize:"15px"}}>{fmt$(sessionCost)}</span>
        <span style={{color:C.faint}}>·</span>
        <span style={{color:C.text,fontWeight:500}}>{sessionRuns} run{sessionRuns!==1?"s":""}</span>
      </>)}
      <span style={{marginLeft:"auto",color:C.muted,fontSize:"12px"}}>Total = Haiku + billable APIs (Voyage · OpenAlex)</span>
    </div>
  );
}

function APIUsageBar({apiUsage}) {
  if(!apiUsage?.providers?.length) return null;
  const hot=p=>p.limit>0&&p.pct_used>=80;
  return (
    <div style={{background:"#0c1018",borderBottom:`1px solid ${C.border}`,padding:"12px 40px 14px",fontSize:"12px",color:C.secondary}}>
      <div style={{display:"flex",alignItems:"center",gap:"12px",marginBottom:"10px",flexWrap:"wrap"}}>
        <span style={{color:C.label,letterSpacing:"0.08em",textTransform:"uppercase",fontSize:"11px",fontWeight:700}}>API usage</span>
        <span style={{color:C.muted}}>resets daily UTC · saved in history.json</span>
        {apiUsage.daily_reset_utc&&<span style={{color:C.faint}}>· {apiUsage.daily_reset_utc}</span>}
      </div>
      <div style={{display:"flex",flexWrap:"wrap",gap:"12px 20px"}}>
        {apiUsage.providers.map(p=>{
          const warn=hot(p);
          const pct=p.limit>0?Math.min(100,p.pct_used||0):0;
          const accent=warn?"#fb923c":"#60a5fa";
          return (
            <div key={p.id} style={{minWidth:"160px",maxWidth:"240px",flex:"1 1 160px"}} title={p.note||""}>
              <div style={{display:"flex",justifyContent:"space-between",marginBottom:"5px",gap:"8px",alignItems:"baseline"}}>
                <span style={{color:warn?"#fdba74":C.text,fontWeight:600,fontSize:"12px",overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap"}}>{p.display_name}</span>
                <span style={{color:warn?"#fdba74":C.secondary,fontWeight:700,fontSize:"12px",flexShrink:0}}>
                  {p.limit>0
                    ? `${fmtUsage(p.period==="account"?p.used_lifetime:p.used_daily,p.unit)}/${fmtUsage(p.limit,p.unit)}`
                    : `${fmtUsage(p.used_lifetime,p.unit)}`}
                </span>
              </div>
              {p.limit>0&&(
                <div style={{height:6,background:"#252a35",borderRadius:3,overflow:"hidden"}}>
                  <div style={{height:"100%",width:`${Math.max(pct, pct>0?4:0)}%`,background:accent,borderRadius:3,transition:"width 0.3s"}}/>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ── Batch info panel ──────────────────────────────────────────────────────────
function BatchInfoPanel({estimate,onToggleOff}) {
  if(!estimate) return null;
  const aux=estimate.est_aux_cost_usd||0;
  const batchTotal=estimate.est_cost_batch_usd||0;
  const haikuSync=haikuFromEstimate(estimate);
  const haikuBatch=haikuBatchFromEstimate(estimate);
  const haikuSavings=haikuSync-haikuBatch;
  const savingsPct=haikuSync>0?Math.round(haikuSavings/haikuSync*100):50;
  return (
    <div style={{padding:"16px 20px",background:"rgba(96,165,250,0.05)",border:"1px solid rgba(96,165,250,0.2)",borderRadius:"8px",marginBottom:"14px"}}>
      <div style={{display:"flex",justifyContent:"space-between",alignItems:"flex-start",marginBottom:"12px"}}>
        <div style={{fontSize:"12px",color:"#60a5fa",fontWeight:600,letterSpacing:"0.05em"}}>BATCH MODE — ANTHROPIC BATCH API</div>
        <button onClick={onToggleOff} style={{...S.ghost,fontSize:"10px",padding:"2px 8px"}}>switch to instant</button>
      </div>
      <div style={{display:"grid",gridTemplateColumns:"1fr 1fr 1fr",gap:"12px",marginBottom:"12px"}}>
        <div style={{textAlign:"center",padding:"10px",background:"#0a0c10",borderRadius:"6px",border:"1px solid #1e2536"}}>
          <div style={{fontSize:"18px",color:"#60a5fa",fontWeight:700,marginBottom:"2px"}}>{fmt$(batchTotal)}</div>
          <div style={{fontSize:"10px",color:C.muted,textTransform:"uppercase",letterSpacing:"0.1em"}}>Batch total</div>
          {aux>0&&<div style={{fontSize:"10px",color:C.faint,marginTop:"4px"}}>Haiku {fmt$(haikuBatch)} + APIs {fmt$(aux)}</div>}
        </div>
        <div style={{textAlign:"center",padding:"10px",background:"#0a0c10",borderRadius:"6px",border:"1px solid #1e2536"}}>
          <div style={{fontSize:"18px",color:"#4ade80",fontWeight:700,marginBottom:"2px"}}>-{savingsPct}%</div>
          <div style={{fontSize:"10px",color:C.muted,textTransform:"uppercase",letterSpacing:"0.1em"}}>Haiku savings (instant Haiku {fmt$(haikuSync)})</div>
        </div>
        <div style={{textAlign:"center",padding:"10px",background:"#0a0c10",borderRadius:"6px",border:"1px solid #1e2536"}}>
          <div style={{fontSize:"18px",color:"#fb923c",fontWeight:700,marginBottom:"2px"}}>≤24h</div>
          <div style={{fontSize:"10px",color:C.muted,textTransform:"uppercase",letterSpacing:"0.1em"}}>Processing time</div>
        </div>
      </div>
      <div style={{fontSize:"12px",color:C.secondary,lineHeight:1.7}}>
        Claims are submitted to Anthropic's asynchronous Batch API. While this tab stays open, status is polled every 8 seconds. When complete, results are saved to **Past fact-checks**. If you refresh the page mid-batch, polling stops — check history after a few minutes or wait for the run to finish on the server.
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
        <div style={{fontSize:"13px",color:C.text,display:"flex",alignItems:"center",gap:"8px",fontWeight:500}}>
          <Spinner size={10} color={isBatch?"#fb923c":"#60a5fa"}/>
          <span>{isBatch?"Batch processing (Anthropic API)…":"Scoring claims in real-time…"}</span>
        </div>
        <span style={{fontSize:"13px",color:C.text,fontWeight:700}}>{done}/{total} · {pct}%</span>
      </div>
      <div style={{background:"#252a35",borderRadius:"4px",height:"8px",overflow:"hidden",marginBottom:"8px"}}>
        <div style={{height:"100%",width:`${pct}%`,background:isBatch?"linear-gradient(90deg,#c2410c,#fb923c)":"linear-gradient(90deg,#1d4ed8,#60a5fa)",transition:"width 0.4s ease",borderRadius:"4px"}}/>
      </div>
      {current&&<div style={{fontSize:"12px",color:C.secondary,fontStyle:"italic",overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap"}}>→ {current}</div>}
      {isBatch&&<div style={{marginTop:"8px",fontSize:"12px",color:C.muted}}>Keep this tab open to watch progress. Finished runs appear under Past fact-checks.</div>}
    </div>
  );
}

// ── History panel ─────────────────────────────────────────────────────────────
function fmtDateFull(s) {
  const d = new Date(s);
  return d.toLocaleDateString(undefined, { year:"numeric", month:"short", day:"numeric" })
    + " " + d.toLocaleTimeString(undefined, { hour:"2-digit", minute:"2-digit" });
}

function HistoryPanel({token,onLoad,refreshKey}) {
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

  useEffect(()=>{
    if(refreshKey===0) return;
    if(open) fetchHistory();
    else setRuns(null); // mark stale so next open re-fetches
  },[refreshKey]);

  function toggle() {
    if(!open&&runs===null) fetchHistory();
    setOpen(o=>!o);
  }

  async function deleteRun(id) {
    await fetch(`${API}/history/${id}`,{method:"DELETE",headers:authH(token)}).catch(()=>{});
    setRuns(rs=>rs.filter(r=>r.id!==id));
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
        {runs!==null&&<span style={{color:C.muted,marginLeft:"4px",fontWeight:600}}>({runs.length})</span>}
      </button>

      {open&&(
        <div style={{marginTop:"10px",...S.card,padding:"0",overflow:"hidden"}}>
          {loading&&<div style={{padding:"16px",fontSize:"13px",color:C.secondary,display:"flex",gap:"8px",alignItems:"center"}}><Spinner size={10}/>Loading…</div>}
          {runs&&runs.length===0&&<div style={{padding:"16px",fontSize:"13px",color:C.muted}}>No past runs yet.</div>}
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
                      <div style={{fontSize:"14px",color:C.text,fontWeight:500,overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap",marginBottom:"4px"}}>
                        {displayName(run)}
                      </div>
                    )}

                    {/* Metadata row */}
                    <div style={{display:"flex",gap:"6px",alignItems:"center",flexWrap:"wrap",marginTop:"2px"}}>
                      <span style={{fontSize:"12px",color:C.muted}}>{fmtDateFull(run.created_at)}</span>
                      <span style={{fontSize:"10px",color:C.faint}}>·</span>
                      <span style={{fontSize:"12px",color:run.mode==="batch"?"#fb923c":"#60a5fa",fontWeight:600}}>{run.mode}</span>
                      <span style={{fontSize:"10px",color:C.faint}}>·</span>
                      <span style={{fontSize:"12px",color:C.secondary}}>{run.claims?.length||0} claims</span>
                      {flagged>0&&<><span style={{fontSize:"10px",color:C.faint}}>·</span><span style={{fontSize:"12px",color:"#f87171",fontWeight:600}}>{flagged} flagged</span></>}
                      {run.filename&&!run.label&&<><span style={{fontSize:"10px",color:C.faint}}>·</span><span style={{fontSize:"12px",color:C.muted}}>📄 {run.filename}</span></>}
                    </div>
                  </div>

                  {/* Right side: cost + rename + delete */}
                  <div style={{display:"flex",flexDirection:"column",alignItems:"flex-end",gap:"4px",flexShrink:0}}>
                    <span style={{fontSize:"13px",color:"#4ade80",fontWeight:700}}>{fmt$(run.cost?.exact_cost_usd||0)}</span>
                    {!isEditing&&(
                      <div style={{display:"flex",gap:"4px"}}>
                        <button onClick={e=>{e.stopPropagation();setEditing(run.id);setEditVal(run.label||"");}}
                          style={{...S.ghost,fontSize:"10px",padding:"2px 6px",letterSpacing:"0.05em"}}>
                          rename
                        </button>
                        <button onClick={e=>{e.stopPropagation();deleteRun(run.id);}}
                          style={{...S.ghost,fontSize:"10px",padding:"2px 6px",color:"#f87171",borderColor:"rgba(248,113,113,0.3)"}}>
                          del
                        </button>
                      </div>
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
function ClaimsView({claims,input,lastCost,fromHistory}) {
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

  function riskStyle(risk) {
    return RISK[risk] ?? RISK.unverifiable;
  }

  const segments=fromHistory?[]:buildSegments();
  const hasAnnotated=!fromHistory&&segments.some(p=>p.type==="claim");
  const counts=claims.reduce((a,c)=>({...a,[c.risk]:(a[c.risk]||0)+1}),{});

  return (
    <div style={{animation:"fadeIn 0.4s ease"}}>
      <div style={{display:"flex",gap:"8px",flexWrap:"wrap",marginBottom:"20px",alignItems:"center"}}>
        <span style={{fontSize:"12px",color:C.label,letterSpacing:"0.1em",textTransform:"uppercase",marginRight:"4px",fontWeight:700}}>{claims.length} claims</span>
        {Object.entries(RISK).map(([key,val])=>counts[key]?(
          <span key={key} style={{display:"inline-flex",alignItems:"center",gap:"5px",fontSize:"12px",fontWeight:600,color:val.dot,background:val.bg,border:`1px solid ${val.border}`,borderRadius:"20px",padding:"4px 12px"}}>
            <span style={{width:6,height:6,borderRadius:"50%",background:val.dot,display:"inline-block"}}/>
            {counts[key]} {val.label}
          </span>
        ):null)}
        {lastCost&&<span style={{marginLeft:"auto"}}><CostSummary cost={lastCost} label="exact"/></span>}
      </div>

      {fromHistory&&<div style={{...S.card,fontSize:"13px",color:C.muted,marginBottom:"20px",padding:"12px 16px"}}>Loaded from history — original input text is not stored; claim breakdown below is complete.</div>}

      {hasAnnotated&&(
      <div style={{...S.card,fontSize:"15px",lineHeight:"1.9",marginBottom:"20px"}}>
        {segments.map((p,i)=>p.type==="plain"?<span key={i}>{p.text}</span>:(
          <span key={i} onClick={()=>setActiveIdx(activeIdx===p.idx?null:p.idx)}
            style={{background:riskStyle(p.risk).bg,borderBottom:`2px solid ${riskStyle(p.risk).border}`,borderRadius:"2px",cursor:"pointer",padding:"1px 2px",outline:activeIdx===p.idx?`2px solid ${riskStyle(p.risk).border}`:"none",outlineOffset:"1px"}}>
            {p.text}
          </span>
        ))}
      </div>
      )}

      {/* Claim cards */}
      <div style={{fontSize:"12px",letterSpacing:"0.12em",color:C.label,fontWeight:700,textTransform:"uppercase",marginBottom:"10px"}}>Claim Breakdown</div>
      <div style={{display:"flex",flexDirection:"column",gap:"8px"}}>
        {claims.map((c,i)=>{
          const col=RISK[c.risk]??RISK.unverifiable;
          const isActive=activeIdx===i;
          return (
            <div key={i} style={{border:`1px solid ${isActive?col.border:"#1e2536"}`,borderLeft:`3px solid ${col.border}`,borderRadius:"6px",padding:"12px 16px",background:isActive?col.bg:"#0a0c10",transition:"all 0.15s"}}>
              <div style={{display:"flex",alignItems:"flex-start",gap:"10px",cursor:"pointer"}} onClick={()=>setActiveIdx(isActive?null:i)}>
                <span style={{fontSize:"10px",fontWeight:700,letterSpacing:"0.1em",color:col.dot,textTransform:"uppercase",minWidth:"90px",paddingTop:"2px"}}>{col.label}</span>
                <div style={{flex:1}}>
                  <div style={{fontSize:"13px",color:C.text,marginBottom:"4px",lineHeight:"1.5"}}>"{c.text.length>140?c.text.slice(0,140)+"…":c.text}"</div>
                  <div style={{fontSize:"13px",color:C.secondary,lineHeight:"1.6"}}>{c.explanation}</div>
                </div>
              </div>
              {c.sources?.length>0&&(
                <div style={{marginTop:"10px",paddingTop:"10px",borderTop:"1px solid #131926"}}>
                  <div style={{fontSize:"11px",color:C.label,fontWeight:700,letterSpacing:"0.08em",textTransform:"uppercase",marginBottom:"6px"}}>Sources ({c.sources.length})</div>
                  <div style={{display:"flex",flexDirection:"column",gap:"6px"}}>
                    {c.sources.map((s,j)=>(
                      <div key={j} style={{display:"flex",gap:"8px",alignItems:"flex-start"}}>
                        <span style={{fontSize:"11px",color:C.muted,paddingTop:"3px",flexShrink:0}}>↗</span>
                        <div>
                          <a href={s.url} target="_blank" rel="noreferrer"
                            style={{fontSize:"12px",color:"#60a5fa",textDecoration:"none",display:"block",lineHeight:"1.4"}}
                            onMouseEnter={e=>e.target.style.textDecoration="underline"}
                            onMouseLeave={e=>e.target.style.textDecoration="none"}>
                            {s.title}
                          </a>
                          {s.provider&&<span style={{fontSize:"10px",color:C.faint,letterSpacing:"0.06em",textTransform:"uppercase"}}>{s.provider.replace("_"," ")}</span>}
                          {s.snippet&&<div style={{fontSize:"12px",color:C.muted,lineHeight:"1.5",marginTop:"3px"}}>{s.snippet.length>180?s.snippet.slice(0,180)+"…":s.snippet}</div>}
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
  // Batch async job tracked via progress state only
  // Persistent cost totals (loaded from /history on login)
  const [allTimeCost,setAllTimeCost] = useState(0);
  const [allTimeIn,setAllTimeIn]     = useState(0);
  const [allTimeOut,setAllTimeOut]   = useState(0);
  const [apiUsage,setApiUsage]       = useState(null);
  // Session counters
  const [sessionCost,setSessionCost] = useState(0);
  const [sessionRuns,setSessionRuns] = useState(0);
  // Trigger history refresh after each completed run
  const [histRefreshKey,setHistRefreshKey] = useState(0);
  const [fromHistory,setFromHistory] = useState(false);
  const [fileLoading,setFileLoading] = useState(false);

  const fileRef = useRef(null);
  const estTimer = useRef(null);
  const batchPollRef = useRef(null);

  function clearBatchPoll() {
    if (batchPollRef.current) {
      clearInterval(batchPollRef.current);
      batchPollRef.current = null;
    }
  }

  useEffect(() => () => clearBatchPoll(), []);

  // Load all-time totals on login
  useEffect(()=>{
    if(!token) return;
    fetch(`${API}/history`,{headers:authH(token)}).then(r=>r.ok?r.json():null).then(d=>{
      if(d){
        setAllTimeCost(d.total_cost_usd||0);
        setAllTimeIn(d.total_input_tokens||0);
        setAllTimeOut(d.total_output_tokens||0);
        if(d.api_usage) setApiUsage(d.api_usage);
      }
    }).catch(()=>{});
  },[token,histRefreshKey]);

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
    clearBatchPoll();
    await fetch(`${API}/logout`,{method:"POST",headers:authH(token)}).catch(()=>{});
    sessionStorage.removeItem("fc_token"); setToken(null);
    setClaims(null); setInput(""); setFileName(null); setLabel(""); setEstimate(null); setLastCost(null);
  }

  async function handleFileChange(e){
    const f=e.target.files?.[0]; if(!f) return;
    setFileName(f.name);
    if(!label) setLabel(f.name.replace(/\.[^.]+$/,""));
    if(f.size>15000) setBatch(true);
    setClaims(null); setFromHistory(false); setError(null);
    const ext=(f.name.match(/\.[^.]+$/)||[""])[0].toLowerCase();
    const plain=[".txt",".md"];
    if(plain.includes(ext)){
      const reader=new FileReader();
      reader.onload=ev=>{setInput(ev.target.result);};
      reader.readAsText(f);
      return;
    }
    setFileLoading(true);
    try {
      const fd=new FormData();
      fd.append("file",f);
      const r=await fetch(`${API}/extract/file`,{method:"POST",headers:{Authorization:`Bearer ${token}`},body:fd});
      if(r.status===401){sessionStorage.removeItem("fc_token");setToken(null);return;}
      if(!r.ok) throw new Error(await r.text()||`Extract failed (${r.status})`);
      const d=await r.json();
      setInput(d.text||"");
      if(d.filename) setFileName(d.filename);
    } catch(err){ setError(err.message||"Could not read file."); setFileName(null); setInput(""); }
    finally { setFileLoading(false); }
  }

  function onCostRecorded(cost){
    setLastCost(cost);
    setSessionCost(s=>s+(cost?.exact_cost_usd||0));
    setSessionRuns(s=>s+1);
    setHistRefreshKey(k=>k+1);
  }

  async function analyze(){
    if(!input.trim()) return;
    clearBatchPoll();
    setLoading(true); setError(null); setClaims(null); setLastCost(null); setProgress(null); setFromHistory(false);

    if(batch){
      try {
        const r=await fetch(`${API}/analyze`,{method:"POST",headers:authH(token),body:JSON.stringify({text:input,batch:true,label,filename:fileName||""})});
        if(r.status===401){sessionStorage.removeItem("fc_token");setToken(null);setLoading(false);return;}
        if(r.status===402){setError(await r.text());setLoading(false);return;}
        if(!r.ok) throw new Error(`Server error: ${r.status}`);
        const d=await r.json();
        if(d.batch_id){
          setProgress({done:0,total:0,current:"Submitting batch…",mode:"batch"});
          pollBatch(d.batch_id);
          return;
        }
        setError("Unexpected response from batch submit.");
        setLoading(false);
      } catch(e){setError(e.message);setLoading(false);}
      return;
    }
    let streamDone=false;
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
        let eventType=""; let dataLine="";

        while(true){
          const {done,value}=await reader.read();
          if(done) break;
          buf+=decoder.decode(value,{stream:true});
          const lines=buf.split("\n");
          buf=lines.pop();
          for(const line of lines){
            if(line.startsWith("event:")) eventType=line.slice(6).trim();
            else if(line.startsWith("data:")) dataLine=line.slice(5).trim();
            else if(line===""&&eventType&&dataLine){
              try {
                const payload=JSON.parse(dataLine);
                if(eventType==="status") setProgress(p=>({done:p?.done||0,total:p?.total||0,current:payload.message||"",mode:"sync"}));
                else if(eventType==="extracted") setProgress({done:0,total:payload.total,current:payload.topic?`Topic: ${payload.topic}`:"",mode:"sync"});
                else if(eventType==="progress") setProgress(p=>({...p,done:payload.done,total:payload.total,current:payload.current,mode:"sync"}));
                else if(eventType==="done"){
                  streamDone=true;
                  setClaims(payload.claims||[]);
                  if(payload.cost) onCostRecorded(payload.cost);
                  setProgress(null); setLoading(false);
                }
                else if(eventType==="error"){ streamDone=true; setError(payload.message); setProgress(null); setLoading(false); }
              } catch {}
              eventType=""; dataLine="";
            }
          }
        }
        if(!streamDone){ setError("Connection closed before analysis finished."); setProgress(null); setLoading(false); }
      } catch(e){ setError(e.message); setProgress(null); setLoading(false); }
  }

  async function pollBatch(batchId){
    clearBatchPoll();
    const tick=async()=>{
      try {
        const r=await fetch(`${API}/batch/${batchId}`,{headers:authH(token)});
        if(r.status===404){
          clearBatchPoll();
          setError("Batch job not found (server may have restarted). Check Past fact-checks — the run may already be saved.");
          setLoading(false); setProgress(null);
          return;
        }
        if(!r.ok){
          clearBatchPoll();
          setError("Batch status check failed.");
          setLoading(false); setProgress(null);
          return;
        }
        const d=await r.json();
        setProgress({done:d.succeeded||0,total:d.total||0,current:"",mode:"batch"});
        if(d.status==="done"){
          clearBatchPoll();
          setClaims(d.claims||[]);
          if(d.cost) onCostRecorded(d.cost);
          setLoading(false); setProgress(null);
        } else if(d.status==="failed"){
          clearBatchPoll();
          setError(d.error||"Batch failed.");
          setLoading(false); setProgress(null);
        }
      } catch(e){
        clearBatchPoll();
        setError(e.message);
        setLoading(false); setProgress(null);
      }
    };
    await tick();
    batchPollRef.current=setInterval(tick,8000);
  }

  function loadHistoryRun(run){
    setInput("");
    setLabel(run.label||"");
    setFileName(run.filename||null);
    setClaims(run.claims);
    setLastCost(run.cost);
    setProgress(null); setError(null); setFromHistory(true);
  }

  if(!token) return <Login onLogin={handleLogin}/>;

  const isLarge=input.length>3000;
  const estPill = estimate&&(
    <span style={{fontSize:"11px",color:"#94a3b8",background:"#141820",border:`1px solid ${C.border}`,borderRadius:"20px",padding:"4px 12px",display:"inline-flex",alignItems:"center",gap:"6px"}}
      title={estimate.aux_costs?.length?auxCostTitle(estimate.aux_costs):undefined}>
      <span style={{width:6,height:6,borderRadius:"50%",background:"#60a5fa",display:"inline-block"}}/>
      ~{estimate.estimated_claims} claims · <span style={{color:"#60a5fa",fontWeight:600}}>{fmt$(batch?estimate.est_cost_batch_usd:estimate.est_cost_usd)}</span>
      {(estimate.est_aux_cost_usd||0)>0.00001&&<span style={{color:C.muted}}>(Haiku {fmt$(haikuFromEstimate(estimate))} + APIs {fmt$(estimate.est_aux_cost_usd)})</span>}
      {(estimate.est_aux_cost_usd||0)<=0.00001&&estimate.aux_costs?.some(l=>l.note)&&<span style={{color:C.muted}}>(APIs free tier)</span>}
    </span>
  );

  return (
    <div style={{minHeight:"100vh",background:"#0d0f14",fontFamily:"'DM Mono','Fira Mono','Courier New',monospace",color:"#e2e8f0"}}>
      <style>{`
        @keyframes spin{to{transform:rotate(360deg)}}
        @keyframes fadeIn{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
        input[type=file]{display:none}
        textarea::placeholder,input::placeholder{color:#7c8da3;opacity:1}
      `}</style>

      <CostBar allTimeCost={allTimeCost} allTimeIn={allTimeIn} allTimeOut={allTimeOut} sessionCost={sessionCost} sessionRuns={sessionRuns}/>
      <APIUsageBar apiUsage={apiUsage}/>

      {/* Header */}
      <div style={{borderBottom:"1px solid #1e2536",padding:"20px 40px",display:"flex",alignItems:"center",gap:"14px"}}>
        <span style={{fontSize:"11px",letterSpacing:"0.2em",color:C.muted,textTransform:"uppercase"}}>VERIFY</span>
        <h1 style={{margin:0,fontSize:"20px",fontWeight:700,fontFamily:"'DM Serif Display',Georgia,serif",color:"#f1f5f9",letterSpacing:"-0.02em"}}>Claim Inspector</h1>
        <span style={{fontSize:"12px",color:C.secondary,marginLeft:"auto"}}>Wikipedia · OpenAlex · Semantic Scholar · PubMed</span>
        <button onClick={handleLogout} style={{...S.ghost}}>sign out</button>
      </div>

      <div style={{maxWidth:"900px",margin:"0 auto",padding:"32px 40px"}}>

        <HistoryPanel token={token} onLoad={loadHistoryRun} refreshKey={histRefreshKey}/>

        {/* Input */}
        <div style={{marginBottom:"14px"}}>
          <div style={{display:"flex",justifyContent:"space-between",alignItems:"center",marginBottom:"10px"}}>
            <label style={S.label}>Input Text</label>
            <div style={{display:"flex",gap:"8px",alignItems:"center"}}>
              {estPill}
              <button onClick={()=>{setInput(SAMPLE);setFileName(null);setLabel("");setClaims(null);setBatch(false);setFromHistory(false);}} style={S.ghost}>load sample</button>
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
          <div style={{fontSize:"12px",color:C.muted,marginTop:"6px"}}>
            Supported: <span style={{color:C.secondary}}>.txt .md .html .docx .pdf</span>
            <span style={{color:C.faint,margin:"0 8px"}}>·</span>
            <span style={{color:C.faint}}>Scanned PDFs not supported</span>
          </div>
        </div>

        {/* Run label */}
        <div style={{marginBottom:"14px"}}>
          <input value={label} onChange={e=>setLabel(e.target.value)} placeholder="Name this fact-check (optional — shown in history)"
            style={{...S.input,fontSize:"13px",color:label?C.text:C.muted}}/>
        </div>

        {/* Batch info panel or simple toggle */}
        {batch ? (
          <BatchInfoPanel estimate={estimate} onToggleOff={()=>setBatch(false)}/>
        ) : (
          <div style={{marginBottom:"14px"}}>
            {isLarge ? (
              <div style={{padding:"12px 16px",background:"rgba(251,146,60,0.06)",border:"1px solid rgba(251,146,60,0.2)",borderRadius:"8px",display:"flex",alignItems:"center",gap:"12px"}}>
                <span style={{fontSize:"14px"}}>⚠</span>
                <div style={{flex:1,fontSize:"13px",color:C.secondary,lineHeight:1.6}}>
                  Large document — <span style={{color:"#fb923c"}}>batch mode saves {estimate?`~${fmt$(haikuFromEstimate(estimate)-haikuBatchFromEstimate(estimate))} on Haiku`:"~50% on Haiku"}</span> but takes up to 24h via Anthropic's async API.
                </div>
                <Toggle value={batch} onChange={setBatch} label="Use batch" color="#fb923c"/>
              </div>
            ) : (
              <Toggle value={batch} onChange={setBatch} label={`Batch mode — Anthropic async API · 50% cheaper Haiku · up to 24h${estimate?` · saves ${fmt$(haikuFromEstimate(estimate)-haikuBatchFromEstimate(estimate))}`:""}`} color="#60a5fa"/>
            )}
          </div>
        )}

        {/* Analyze button */}
        <div style={{display:"flex",alignItems:"center",gap:"14px",marginBottom:"28px"}}>
          <button onClick={analyze} disabled={loading||fileLoading||!input.trim()}
            style={{...S.btn,background:loading?"#1e2536":"#1d4ed8",color:loading?"#475569":"#e0eaff",cursor:loading||fileLoading||!input.trim()?"not-allowed":"pointer",opacity:!input.trim()?0.5:1}}>
            {loading?<><Spinner/>{batch?"Submitting batch…":"Analyzing…"}</>:fileLoading?<><Spinner/>Reading file…</>:"Analyze Claims"}
          </button>
          {lastCost&&!loading&&(
            <div style={{fontSize:"13px",color:C.secondary,display:"flex",gap:"8px",alignItems:"center",flexWrap:"wrap"}}>
              <CostSummary cost={lastCost} label="Last run"/>
              <span style={{color:C.faint}}>·</span>
              <span style={{color:C.text,fontWeight:500}}>{fmtTok(lastCost.usage.input_tokens)} in / {fmtTok(lastCost.usage.output_tokens)} out</span>
            </div>
          )}
        </div>

        {error&&<div style={{color:"#f87171",fontSize:"13px",marginBottom:"20px",padding:"12px 16px",background:"rgba(248,113,113,0.08)",borderRadius:"6px",border:"1px solid rgba(248,113,113,0.2)"}}>{error}</div>}

        {progress&&<ProgressBar done={progress.done} total={progress.total} current={progress.current} mode={progress.mode}/>}

        {claims&&<ClaimsView claims={claims} input={input} lastCost={lastCost} fromHistory={fromHistory}/>}
      </div>
    </div>
  );
}
