'use strict';

const navigation = [
  ['Monitor', [
    ['overview', 'Overview', '◈'], ['charts', 'Charts', '∿'], ['consumption', 'Consumption', '▥'],
    ['events', 'Events', '☷'], ['messages', 'Messages', '▤'], ['notifications', 'Notifications', '◇'],
  ]],
  ['Controller', [
    ['boiler', 'Boiler', '▣'], ['fuel', 'Hopper & feed', '▧'], ['hot-water', 'Hot water', '◉'],
    ['weather', 'Weather', '☁'], ['solar', 'Solar', '☼'], ['system', 'System', '⚙'],
  ]],
  ['Resources', [['help', 'Help & information', 'ⓘ']]],
];
const groups = {
  boiler: ['Boiler', 'Temperature, heating schedule, and external thermostat.', [
    ['boiler','Temperature & schedule'], ['regulation','Regulation'], ['oxygen','Oxygen'], ['cleaning','Cleaning'],
    ['fan','Fan'], ['ignition','Ignition'], ['pump','Pump'], ['alarm','Alarm'],
  ]],
  fuel: ['Hopper & feed', 'Fuel level, pellet feed, and auger calibration.', [
    ['hopper','Hopper'], ['auger','Augers & external feed'], ['vacuum','Vacuum transport'],
  ]],
  'hot-water': ['Hot water', 'Cylinder temperature, hot water output, and weekly schedule.', [['hot_water','Temperature & schedule']]],
  weather: ['Weather', 'Outdoor readings and weather compensation for both heating circuits.', [
    ['weather','Heating circuit 1'], ['weather2','Heating circuit 2'],
  ]],
  solar: ['Solar', 'Solar collector, cylinder temperatures, and pump settings.', [['sun','Solar circuit']]],
  system: ['System', 'Controller information, live diagnostics, and service settings.', [['misc','Controller'], ['manual','Service']]],
};
const labels = {
  boiler_temp: ['Boiler temperature','°C'], boiler_ref: ['Boiler target','°C'],
  smoke_temp: ['Flue gas','°C'], return_temp: ['Return temperature','°C'],
  shaft_temp: ['Drop shaft','°C'], dhw_temp: ['Hot water','°C'], dhw_ref: ['Hot water target','°C'],
  oxygen: ['Oxygen','%'], oxygen_ref: ['Oxygen target','%'], power_kw: ['Heat output','kW'],
  power_pct: ['Burner power','%'], content: ['Pellets remaining','kg'], distance: ['Hopper distance','cm'],
  external_temp: ['Outdoor sensor','°C'], outdoor_temp: ['Outdoor weather','°C'], wind_speed: ['Wind speed','m/s'],
  wind_direction: ['Wind direction',''], air_pressure: ['Atmospheric pressure','hPa'], cloud_level: ['Cloud cover','%'],
  humidity: ['Relative humidity','%'], forward_temp: ['Circuit 1 supply','°C'], forward2_temp: ['Circuit 2 supply','°C'],
  sun_temp: ['Solar collector','°C'], sun2_temp: ['Second collector','°C'], sun_dhw_temp: ['Solar tank','°C'],
  sun_power_kw: ['Solar output','kW'], sun_pumpspeed: ['Solar pump speed','%'], fan_speed: ['Fan speed','%'],
  total_consumption_24hours: ['Pellets in the last 24 hours','kg'], dhw_consumption_24hours: ['Hot-water fuel in the last 24 hours','kg'],
  internet_uptime: ['Controller online time','%'], program_version: ['Firmware version',''], photo_level: ['Flame sensor',''],
};
const colors = ['#345ece','#118576','#b25e35','#a65095','#5c7e36','#4685ad','#ab7f2e','#64748b'];
const days = ['monday','tuesday','wednesday','thursday','friday','saturday','sunday'];
const datetime = new Intl.DateTimeFormat(undefined,{dateStyle:'medium',timeStyle:'short'});
const shorttime = new Intl.DateTimeFormat(undefined,{hour:'numeric',minute:'2-digit'});
const numeric = new Intl.NumberFormat(undefined,{maximumFractionDigits:2});
const main = document.querySelector('main');
let status = {data:{},online:false,control:false};
let page = '';
let generation = 0;
let toastTimer = 0;
let noticeReady = false;
let lastDesktopNotice = 0;
let noticeFilter = saved('noticeFilter', ['alarm','info','connection']);
let chosenSignals = saved('signals',['operating_data.boiler_temp','operating_data.boiler_ref']);
let chosenMetrics = saved('metrics',['operating_data.oxygen','operating_data.smoke_temp','operating_data.external_temp','operating_data.air_flow']);

function dom(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key,value] of Object.entries(attrs)) {
    if (value == null || value === false) continue;
    if (key === 'class') node.className = value;
    else if (key.startsWith('on')) node.addEventListener(key.slice(2), value);
    else node.setAttribute(key, value === true ? '' : value);
  }
  for (const child of children.flat(Infinity)) {
    if (child != null && typeof child !== 'boolean') node.append(child);
  }
  return node;
}
function saved(key, fallback) {
  try { return JSON.parse(localStorage.getItem('boilermate.'+key)) ?? fallback; } catch { return fallback; }
}
function saveLocal(key,value) {
  try { localStorage.setItem('boilermate.'+key,JSON.stringify(value)); } catch {}
}
function textLabel(key) {
  return labels[key]?.[0] ?? key.replaceAll('_',' ').replace(/^./,ch=>ch.toUpperCase());
}
function display(value) {
  if (value == null || value === '') return '—';
  return typeof value === 'number' ? numeric.format(value) : String(value);
}
function snapshot(group) { return status.data[group]?.values ?? {}; }
function signal(key) {
  const [group,name] = key.split('.');
  const value = snapshot(group)[name];
  if (labels[name]?.[1] === '°C' && typeof value === 'number' && value >= 999) return null;
  return value;
}
function titleSignal(key) { return textLabel(key.split('.').at(-1)); }
function unitSignal(key) { return labels[key.split('.').at(-1)]?.[1] ?? ''; }
function heading(title,description,actions) {
  return dom('div',{class:'page-heading'},dom('div',{},dom('h1',{},title),description && dom('p',{},description)),actions);
}
function card(title,description,...content) {
  return dom('section',{class:'card'},dom('div',{class:'card-head'},dom('div',{},dom('h2',{},title),description && dom('p',{},description))),content);
}
function pill(text,kind='') { return dom('span',{class:'pill '+kind},text); }
function empty(text) { return dom('p',{class:'empty'},text); }
function btn(label,callback,kind='secondary',attrs={}) {
  return dom('button',{type:'button',class:kind,onclick:callback,...attrs},label);
}
function toast(message) {
  const el = document.getElementById('toast');
  el.textContent = message;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(()=>{el.textContent='';},5200);
}
async function api(path, options={}) {
  const response = await fetch('/api/'+path,{...options,headers:{'X-Boiler-Mate':'1',...(options.body ? {'Content-Type':'application/json'} : {})}});
  if (response.status === 204) return null;
  if (!response.ok) {
    const error = await response.json().catch(()=>({}));
    throw new Error(error.error ?? 'The service is unavailable.');
  }
  return response.json();
}
function confirmChange(title,message,label) {
  const dialog = document.getElementById('confirm');
  document.getElementById('confirm-title').textContent = title;
  document.getElementById('confirm-text').textContent = message;
  document.getElementById('confirm-action').textContent = label;
  dialog.returnValue = 'cancel';
  dialog.showModal();
  return new Promise(resolve=>{
    dialog.addEventListener('close',()=>resolve(dialog.returnValue==='confirm'),{once:true});
  });
}
function renderNavigation() {
  document.getElementById('navigation').replaceChildren(...navigation.map(([name,links]) =>
    dom('div',{class:'nav-group'},dom('div',{class:'nav-label'},name),
      links.map(([id,label])=>dom('a',{href:'#'+id,class:'side-link','aria-current':id===page?'page':null},label)))));
  const mobile = document.getElementById('mobile-navigation');
  mobile.replaceChildren(...navigation.map(([name,links]) =>
    dom('optgroup',{label:name},links.map(([id,label])=>dom('option',{value:id,selected:id===page},label)))));
  mobile.onchange = () => { window.location.hash = '#'+mobile.value; };
  const label = navigation.flatMap(([,links])=>links).find(([id])=>id===page)?.[1];
  document.getElementById('breadcrumb').textContent = label ?? 'Overview';
  document.title = (label ?? 'Overview')+' · Boiler Mate';
}
function refreshChrome() {
  document.getElementById('sidebar-indicator').classList.toggle('live',Boolean(status.online));
  document.getElementById('sidebar-status').textContent = status.online ? 'Controller connected' : 'Controller unavailable';
  document.getElementById('serial').textContent = status.serial ? 'NBE '+status.serial : '';
  const access = document.getElementById('access');
  access.textContent = status.control ? 'Controls enabled' : 'Read only';
  access.className = 'pill '+(status.control?'good':'warn');
}
async function refresh() {
  try { status = await api('status'); }
  catch { status = {...status,online:false,control:false}; }
  refreshChrome();
  if (page === 'overview') updateOverview();
}
function metric(key, sub='') {
  return dom('article',{class:'card metric'},dom('p',{class:'metric-label'},titleSignal(key)),
    dom('div',{class:'metric-reading'},display(signal(key)),unitSignal(key)&&dom('small',{},unitSignal(key))),
    sub&&dom('p',{class:'metric-sub'},sub));
}
function updateOverview() {
  const live = document.getElementById('overview-live');
  if (!live) return;
  const op = snapshot('operating_data');
  const power = op.state_on === 'OFF' ? 'start' : 'stop';
  const action = power==='start'?'Start boiler':'Stop boiler';
  const command = async ()=>{
    if (!await confirmChange(action+'?', 'This sends a '+power+' command to the pellet boiler.', action)) return;
    try { await api('power',{method:'POST',body:JSON.stringify({action:power})}); toast('Controller acknowledged the command.'); await refresh(); }
    catch(error) { toast(error.message); }
  };
  const last = status.data.operating_data?.at;
  const restoreFocus = live.querySelector('button') === document.activeElement;
  const remaining = signal('operating_data.content');
  const output = signal('operating_data.power_kw');
  const water = signal('operating_data.dhw_temp');
  const returnTemp = signal('operating_data.return_temp');
  const readings = [
    ['Fuel hopper',remaining!=null&&remaining<0 ? 'Below minimum' : withUnit(remaining,'kg'),withUnit(signal('advanced_data.total_consumption_24hours'),'kg')+' used in 24 h','#fuel'],
    ['Boiler',withUnit(output,'kW'),withUnit(signal('operating_data.power_pct'),'%')+' burner output','#boiler'],
    ['Heating circuit',withUnit(returnTemp,'°C'),'Return water','#weather'],
    ['Hot water',water==null?'No reading':withUnit(water,'°C'),'Cylinder sensor','#hot-water'],
  ];
  live.replaceChildren(dom('section',{class:'overview-system','aria-label':'Current boiler state'},
    dom('div',{class:'system-main'},
      dom('div',{},dom('div',{class:'operating-mode'},dom('span',{class:'indicator'+(status.online?' live':'')}),status.online?'Controller online':'Controller offline'),
        dom('h2',{},op.state_text || 'Waiting for data'),
        last&&dom('p',{class:'muted'},'Updated '+shorttime.format(new Date(last)))),
      dom('div',{class:'system-temperature'},dom('span',{},'Boiler temperature'),
        dom('strong',{},display(signal('operating_data.boiler_temp')),dom('small',{},'°C')),
        dom('span',{},'Target '+withUnit(signal('operating_data.boiler_ref'),'°C'))),
      btn(action,command,power==='stop'?'danger':'secondary',{disabled:!status.control||!status.online||!op.state_on})),
    dom('div',{class:'system-line'},readings.map(([label,value,detail,link]) =>
      dom('a',{class:'system-reading',href:link},dom('span',{},label),dom('strong',{},value),dom('small',{},detail))))));
  const custom = document.getElementById('overview-custom');
  if (custom) custom.replaceChildren(...chosenMetrics.map(key=>{
    const value=signal(key);
    return dom('div',{class:'compact-reading'},dom('span',{},titleSignal(key)),
      dom('strong',{},withUnit(value,unitSignal(key))));
  }));
  if (restoreFocus) live.querySelector('button').focus({preventScroll:true});
}
function withUnit(value,unit) {
  return display(value)+(value==null||!unit?'':' '+unit);
}
function overview(token) {
  const live = dom('div',{id:'overview-live'});
  const chartBody = dom('div',{},empty('Loading temperature history…'));
  const recent = dom('div',{},empty('Loading recent messages…'));
  const metricsPanel = dom('div',{},empty('Waiting for telemetry…'));
  const customise = dom('details',{},dom('summary',{},'Choose overview readings'),metricsPanel);
  main.replaceChildren(heading('Pellet boiler'),
    dom('div',{class:'stack'},live,
      dom('div',{class:'grid split'},
        card('Temperature · 24 hours',null,chartBody,dom('a',{class:'home-link',href:'#charts'},'All charts')),
        card('Controller messages',null,recent,dom('a',{class:'home-link',href:'#messages'},'View inbox'))),
      card('Other readings',null,dom('div',{class:'compact-readings',id:'overview-custom'}),customise)));
  updateOverview();
  const renderChooser = ()=>chooseSignals(metricsPanel,chosenMetrics,4,selection=>{
    chosenMetrics=selection;
    saveLocal('metrics',selection);
    updateOverview();
    renderChooser();
  });
  customise.addEventListener('toggle',()=>{if(customise.open)renderChooser();});
  loadHistory(['operating_data.boiler_temp','operating_data.boiler_ref'],86400)
    .then(data=>{if(token===generation) showChart(chartBody,data,86400,true);})
    .catch(error=>chartBody.replaceChildren(empty(error.message)));
  api('notifications').then(items=>{if(token===generation) recent.replaceChildren(noticeList(items.slice(0,4),false));})
    .catch(error=>recent.replaceChildren(empty(error.message)));
}

function measurementKeys() {
  const keys = new Set([...Object.keys(labels).map(key=>'operating_data.'+key),...chosenSignals,...chosenMetrics]);
  for (const group of ['operating_data','advanced_data']) {
    for (const [key,value] of Object.entries(snapshot(group))) {
      if (typeof value==='number' && !['time','state','substate'].includes(key)) keys.add(group+'.'+key);
    }
  }
  for (const key of keys) if (signal(key)==null && !chosenSignals.includes(key) && !chosenMetrics.includes(key)) keys.delete(key);
  return [...keys].sort((a,b)=>titleSignal(a).localeCompare(titleSignal(b))||a.localeCompare(b));
}
function chooseSignals(target,selected,limit,change) {
  const keys = measurementKeys();
  target.replaceChildren(keys.length ? dom('div',{class:'signal-chooser'},keys.map(key=>{
    const checked = selected.includes(key);
    return dom('label',{},dom('input',{type:'checkbox',checked,disabled:!checked&&selected.length>=limit,
      onchange:()=>change(checked?selected.filter(k=>k!==key):[...selected,key])}),titleSignal(key),
      key.startsWith('advanced_data.')?' · Advanced':'');
  })) : empty('Measurements will appear after the first controller reading.'));
}
function loadHistory(keys,duration) {
  const args = new URLSearchParams({from:Math.floor(Date.now()/1000)-duration});
  keys.forEach(key=>args.append('series',key));
  return api('history?'+args);
}
function charts(token) {
  let duration = 86400;
  let request = 0;
  const chartBody = dom('div',{},empty('Loading history…'));
  const signals = dom('div');
  const range = dom('select',{'aria-label':'Chart period',onchange:()=>{duration=Number(range.value);load();}},
    [[21600,'6 hours'],[86400,'24 hours'],[604800,'7 days'],[2592000,'30 days'],[31536000,'1 year'],[378691200,'12 years']]
      .map(([value,label])=>dom('option',{value,selected:value===duration},label)));
  let data;
  const csv = btn('Export CSV',()=>downloadCSV(data),'secondary',{disabled:true});
  const load = async ()=>{
    const id = ++request;
    if (!chosenSignals.length) {
      chartBody.replaceChildren(empty('Choose up to eight signals to chart.'));
      csv.disabled=true; return;
    }
    csv.disabled=true;
    chartBody.replaceChildren(empty('Loading history…'));
    try {
      const loaded = await loadHistory(chosenSignals,duration);
      if(token!==generation||request!==id)return;
      data=loaded;
      showChart(chartBody,data,duration);
      csv.disabled=!Object.values(data).some(points=>points.length);
    } catch(error) {
      if(request===id)chartBody.replaceChildren(empty(error.message));
    }
  };
  const choose = ()=>chooseSignals(signals,chosenSignals,8,selection=>{
    chosenSignals=selection; saveLocal('signals',selection); choose(); load();
  });
  main.replaceChildren(heading('Charts','Compare live signals recorded by Boiler Mate. History starts when this instance is enabled.'),
    card('Signal history','Each signal keeps its own unit.',
      dom('div',{class:'toolbar'},dom('label',{},'Period',range),csv),
      chartBody,dom('details',{open:true},dom('summary',{},'Select signals'),signals)));
  choose(); load();
}

function chartBounds(series) {
  let minX=Infinity,maxX=-Infinity,minY=Infinity,maxY=-Infinity;
  for(const points of series){
    for(const p of points) {
      minX=Math.min(minX,p.time);maxX=Math.max(maxX,p.time);
      minY=Math.min(minY,p.value);maxY=Math.max(maxY,p.value);
    }
  }
  const margin = Math.max((maxY-minY)*.07, .1);
  return {minX,maxX:maxX===minX?minX+1:maxX,minY:minY-margin,maxY:maxY+margin};
}
function showChart(target,data,duration,compact=false) {
  const entries = Object.entries(data).filter(([,points])=>points.length);
  if(!entries.length){target.replaceChildren(empty('No readings in this period yet.'));return;}
  const units = new Set(entries.map(([key])=>unitSignal(key)));
  const mixed = units.size>1;
  const visual = dom('canvas',{class:'chart'+(compact?' compact':''),role:'img','aria-label':'Line chart of '+entries.map(([key])=>titleSignal(key)).join(', ')});
  const legend = dom('div',{class:'legend'},entries.map(([key,points],i)=>{
    const swatch=dom('span',{class:'dot','aria-hidden':'true'});
    swatch.style.backgroundColor=colors[i];
    return dom('span',{},swatch,titleSignal(key)+' · '+display(points.at(-1).value)+' '+unitSignal(key));
  }));
  target.replaceChildren(visual,legend);
  if (mixed) target.append(dom('p',{class:'muted'},'The chart scales each signal independently.'));
  const render = ()=>{
    if(!visual.isConnected)return;
    const width=visual.clientWidth,height=visual.clientHeight,dpr=window.devicePixelRatio||1;
    if(!width||!height)return;
    visual.width=Math.round(width*dpr);visual.height=Math.round(height*dpr);
    const ctx=visual.getContext('2d');
    ctx.scale(dpr,dpr);
    const bounds=chartBounds(entries.map(([,points])=>points));
    const newest=Math.floor(Date.now()/1000);
    bounds.minX=Math.min(bounds.minX,newest-duration);
    bounds.maxX=Math.max(bounds.maxX,newest);
    const step=chartStep((bounds.maxY-bounds.minY)/4);
    bounds.minY=Math.floor(bounds.minY/step)*step;
    bounds.maxY=Math.ceil(bounds.maxY/step)*step;
    const rows=mixed?4:Math.min(8,Math.round((bounds.maxY-bounds.minY)/step));
    const x0=42, x1=width-16, y0=14, y1=height-29;
    ctx.font='11px system-ui';ctx.fillStyle='#556773';
    for(let i=0;i<=rows;i++){
      const y=y1-i*(y1-y0)/rows;
      ctx.strokeStyle='#dce3dd';ctx.lineWidth=1;ctx.beginPath();ctx.moveTo(x0,y);ctx.lineTo(x1,y);ctx.stroke();
      if(!mixed)ctx.fillText(numeric.format(bounds.minY+i*step),2,y+3);
    }
    const formatTime = duration<=86400?shorttime:new Intl.DateTimeFormat(undefined,{month:'short',day:'numeric'});
    const start=formatTime.format(new Date(bounds.minX*1000));
    ctx.fillText(start,x0,height-6);
    const end=formatTime.format(new Date(bounds.maxX*1000));
    ctx.fillText(end,x1-ctx.measureText(end).width,height-6);
    for(const [i,[key,points]] of entries.entries()){
      const own=mixed?chartBounds([points]):bounds;
      ctx.strokeStyle=colors[i];ctx.fillStyle=colors[i];ctx.lineWidth=2.2;ctx.setLineDash(key.endsWith('_ref')?[5,4]:[]);ctx.beginPath();
      for(const [j,p] of points.entries()){
        const x=x0+(p.time-bounds.minX)*(x1-x0)/(bounds.maxX-bounds.minX);
        const y=y1-(p.value-own.minY)*(y1-y0)/(own.maxY-own.minY);
        if(j===0)ctx.moveTo(x,y);else ctx.lineTo(x,y);
        if(points.length===1){ctx.arc(x,y,3,0,Math.PI*2);ctx.fill();}
      }
      ctx.stroke();
    }
  };
  requestAnimationFrame(render);
  window.addEventListener('resize',render,{signal:pageAbort.signal});
}
function chartStep(rough) {
  const scale = 10 ** Math.floor(Math.log10(rough));
  return ([1,2,2.5,5,10].find(step=>step*scale>=rough) ?? 10) * scale;
}
function downloadCSV(data) {
  if(!data)return;
  const csv=['time,signal,value'];
  for(const [key,points] of Object.entries(data)){
    points.forEach(p=>csv.push(new Date(p.time*1000).toISOString()+','+key+','+p.value));
  }
  const url=URL.createObjectURL(new Blob([csv.join('\n')],{type:'text/csv'}));
  const link=dom('a',{href:url,download:'boiler-mate-history.csv'});
  link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
}

function consumption(token) {
  const output=dom('div',{},empty('Reading consumption from the controller…'));
  let active='hours';let sequence=0;
  const periods=[['hours','Hourly'],['days','Daily'],['months','Monthly'],['years','Yearly']];
  const selector=dom('div',{class:'switcher','aria-label':'Consumption period'});
  const load=async period=>{
    active=period;const id=++sequence;
    selector.replaceChildren(...periods.map(([key,label])=>btn(label,()=>load(key),'secondary',{'aria-pressed':key===active?'true':'false'})));
    output.replaceChildren(empty('Reading consumption from the controller…'));
    try {
      const data=await api('consumption?period='+period);
      if(token!==generation||id!==sequence)return;
      const aggregate=data.total.reduce((sum,value)=>sum+value,0);
      const dhw=data.hotWater?.reduce((sum,value)=>sum+value,0);
      output.replaceChildren(dom('div',{class:'grid metrics'},
        stat('Period total',aggregate,'kg'),stat('Hot water',dhw,'kg'),stat('Controller counter',data.counter?.[0],'kg')),
        dom('div',{class:'table-scroll'},consumptionTable(data)));
    } catch(error) {if(id===sequence)output.replaceChildren(empty(error.message),btn('Try again',()=>load(period)));}
  };
  main.replaceChildren(heading('Consumption','Historical pellet use stored in the controller, including the domestic hot-water share when available.'),
    card('Pellet use',null,selector,output));
  load(active);
}
function stat(label,value,unit) {
  return dom('article',{class:'card metric'},dom('p',{class:'metric-label'},label),
    dom('p',{class:'metric-reading'},display(value),unit&&dom('small',{},unit)));
}
function consumptionTable(data) {
  const max = Math.max(...data.total,1);
  return dom('table',{},dom('thead',{},dom('tr',{},
    ['Period','Total (kg)','Hot water (kg)','Heating (kg)'].map(name=>dom('th',{scope:'col'},name)))),
    dom('tbody',{},data.total.map((value,i)=>{
      const hot=data.hotWater?.[i];
      const heat=hot==null ? null : Math.max(0,value-hot);
      return dom('tr',{},dom('th',{scope:'row'},(i+1)+' / '+data.total.length),
        dom('td',{},dom('meter',{value,min:0,max,'aria-label':'Total pellet use'}),' '+display(value)),
        dom('td',{},display(hot)),dom('td',{},display(heat)));
    })));
}

function noticeList(rows,actions) {
  if(!rows.length)return empty('No messages here.');
  return dom('ul',{class:'notice-list'},rows.map(row=>dom('li',{},
    pill(textLabel(row.kind),row.kind==='alarm'?'error':row.kind==='connection'?'warn':''),
    dom('div',{},dom('strong',{},row.message),dom('small',{},datetime.format(new Date(row.time*1000)))),
    actions&&!row.read&&btn('Mark read',async event=>{
      event.currentTarget.disabled=true;
      try {await api('notifications/'+row.id+'/read',{method:'POST'});event.currentTarget?.remove();renderPage();}
      catch(error){toast(error.message);renderPage();}
    },'subtle'))));
}
function activity(token,noticeMode) {
  const title=noticeMode?'Messages':'Events';
  const description=noticeMode?'Local inbox for controller alarms, information, and communication changes.':
    'Boiler state transitions, controller notices, and commands acknowledged by the controller.';
  const entries=dom('div',{},empty('Loading '+title.toLowerCase()+'…'));
  const filter=dom('select',{'aria-label':'Filter '+title.toLowerCase()},
    [['all','All'],['unread','Unread'],['alarm','Alarms'],['info','Information'],['connection','Connection'],
      ...(!noticeMode?[['state','State changes'],['command','Commands']]:[])].map(([key,label])=>dom('option',{value:key},label)));
  const more=btn('Load older',()=>load(true));
  more.hidden=true;
  let rows=[];
  function render() {
    const shown=rows.filter(row=>filter.value==='all'||(filter.value==='unread'?!row.read:row.kind===filter.value));
    entries.replaceChildren(noticeList(shown,noticeMode&&status.lock));
  }
  filter.addEventListener('change',render);
  const load=async older=>{
    more.disabled=true;
    try {
      const endpoint=noticeMode?'notifications':'events';
      const result=await api(endpoint+(older?'?before='+rows.at(-1).id:''));
      if(token!==generation)return;
      rows=older?[...rows,...result]:result;
      more.hidden=result.length<150;
      render();
    } catch(error) {entries.replaceChildren(empty(error.message));}
    finally {more.disabled=false;}
  };
  main.replaceChildren(heading(title,description),
    card(noticeMode?'Inbox':'Recent activity',null,dom('div',{class:'toolbar'},dom('label',{},'Filter',filter)),entries,more));
  load(false);
}
function notifications(token) {
  const result=dom('div',{},empty('Loading recent notifications…'));
  const desktop=btn(typeof Notification!=='undefined'&&Notification.permission==='granted'?'Desktop notifications enabled':'Enable desktop notices',async ()=>{
    if(typeof Notification==='undefined'||!window.isSecureContext){toast('Desktop notices need HTTPS or a localhost connection.');return;}
    const permission=await Notification.requestPermission();
    toast(permission==='granted'?'Desktop notices are enabled.':'Your browser did not grant notification access.');
    if(permission==='granted')desktop.textContent='Desktop notifications enabled';
  },'primary');
  const options=[['alarm','Boiler alarms'],['info','Controller information'],['connection','Connection interruptions']];
  const inputs=dom('div',{class:'signal-chooser'},options.map(([kind,label])=>dom('label',{},
    dom('input',{type:'checkbox',checked:noticeFilter.includes(kind),onchange:event=>{
      noticeFilter=event.target.checked?[...noticeFilter,kind]:noticeFilter.filter(item=>item!==kind);
      saveLocal('noticeFilter',noticeFilter);
    }}),label)));
  main.replaceChildren(heading('Notifications','Choose which controller notices can appear on this browser.'),
    dom('div',{class:'stack'},card('Desktop notifications','Boiler Mate can show notices while this dashboard is open.',inputs,desktop),
      card('Recent notifications',null,result,dom('a',{class:'home-link',href:'#messages'},'Open messages'))));
  api('notifications').then(items=>{if(token===generation)result.replaceChildren(noticeList(items.slice(0,8),status.lock));})
    .catch(error=>result.replaceChildren(empty(error.message)));
}
async function checkDesktopNotices() {
  if(typeof Notification==='undefined'||Notification.permission!=='granted')return;
  try {
    const rows=await api('notifications');
    if(noticeReady)rows.filter(row=>row.id>lastDesktopNotice&&noticeFilter.includes(row.kind)&&!row.read)
      .reverse().forEach(row=>new Notification('Boiler Mate', {body:row.message,tag:'boiler-'+row.id}));
    lastDesktopNotice=Math.max(lastDesktopNotice,rows[0]?.id??0);
    noticeReady=true;
  } catch {}
}
function dataTable(type) {
  const data=snapshot(type);
  const entries=Object.entries(data).sort(([a],[b])=>textLabel(a).localeCompare(textLabel(b)));
  if(!entries.length)return empty('The controller has not returned these readings.');
  return dom('div',{class:'table-scroll'},dom('table',{},
    dom('thead',{},dom('tr',{},dom('th',{scope:'col'},'Signal'),dom('th',{scope:'col'},'Reading'))),
    dom('tbody',{},entries.map(([key,value])=>dom('tr',{},
      dom('th',{scope:'row'},textLabel(key)),dom('td',{},display(value)+' '+(labels[key]?.[1]??'')))))));
}
function settingsPage(id,token) {
  const [title,description,tabs]=groups[id];
  const controls=dom('div',{},empty('Loading settings…'));
  const tablist=dom('div',{class:'switcher','aria-label':title+' settings categories'});
  let selected=tabs[0][0];let sequence=0;
  const editEnabled=()=>status.control&&status.online;
  const metrics={
    boiler:['operating_data.boiler_temp','operating_data.return_temp','operating_data.power_kw','operating_data.smoke_temp'],
    fuel:['operating_data.content','operating_data.distance','advanced_data.total_consumption_24hours','advanced_data.dhw_consumption_24hours'],
    'hot-water':['operating_data.dhw_temp','operating_data.dhw_ref'],
    weather:['advanced_data.outdoor_temp','advanced_data.wind_speed','advanced_data.humidity','operating_data.forward_temp'],
    solar:['operating_data.sun_temp','operating_data.sun2_temp','operating_data.sun_dhw_temp','operating_data.sun_power_kw'],
    system:['advanced_data.program_version','operating_data.internet_uptime'],
  };
  const load=async group=>{
    selected=group;
    const index=++sequence;
    tablist.replaceChildren(...tabs.map(([key,name])=>btn(name,()=>load(key),'secondary',{'aria-pressed':key===selected?'true':'false'})));
    controls.replaceChildren(empty('Loading controller values…'));
    try {
      const fields=await api('settings/'+group);
      if(token!==generation||index!==sequence)return;
      const inputs=dom('div',{class:'fields'});
      const options=fields.filter(field=>!field.timer);
      const update=query=>{
        const match=query.trim().toLowerCase();
        const filtered=options.filter(field=>(field.label+' '+field.key).toLowerCase().includes(match));
        inputs.replaceChildren(...(filtered.length?filtered.map(field=>settingRow(group,field,editEnabled,()=>load(group))):[empty('No settings match.')]));
      };
      const search=dom('input',{type:'search',class:'field-input lookup','aria-label':'Filter settings',placeholder:'Filter settings',oninput:event=>update(event.target.value)});
      const timerFields=fields.filter(field=>field.timer);
      controls.replaceChildren(dom('div',{class:'toolbar'},search),inputs,
        timerFields.length&&timerEditor(group,timerFields,editEnabled,()=>load(group)));
      update('');
    } catch(error) {
      if(index===sequence)controls.replaceChildren(empty(error.message),btn('Try again',()=>load(group)));
    }
  };
  const diagnostics=id==='system' ? [
    card('Live & advanced diagnostics','Includes regulation, combustion, and connected outputs.',
      dom('details',{},dom('summary',{},'Operating data'),dataTable('operating_data')),
      dom('details',{},dom('summary',{},'Advanced data'),dataTable('advanced_data'))),
    card('Alarm reset','Use this after resolving the alarm on the boiler.',
      btn('Reset alarm',async()=>{
        if(!await confirmChange('Reset controller alarm?','Resolve the cause of the alarm before resetting it.','Reset alarm'))return;
        try {await api('alarm/reset',{method:'POST'});toast('Controller acknowledged the alarm reset.');}
        catch(error){toast(error.message);}
      },'danger',{disabled:!editEnabled()})),
  ] : [];
  main.replaceChildren(heading(title,description,btn('Refresh',async()=>{await refresh();renderPage();})),
    dom('div',{class:'stack'},
      dom('div',{class:'grid metrics'},metrics[id].map(key=>metric(key))),
      card('Controller settings',status.control?'Changes are sent to the controller after confirmation.':'You can view settings in read-only mode.',tablist,controls),
      diagnostics));
  load(selected);
}

function settingRow(group,field,canEdit,reload) {
  const range=field.range;
  const id='field-'+group+'-'+field.key;
  const detail=dom('details',{},dom('summary',{},'Help'),dom('p',{},field.help),
    range&&dom('p',{},'Range '+display(range.min)+' to '+display(range.max)+' · Default '+display(range.default)));
  const label=dom('div',{class:'field-label'},dom('label',{for:id,class:'field-name'},field.label),
    dom('small',{class:'field-id'},group+'.'+field.key),detail);
  if(!range||!canEdit())return dom('div',{class:'field'},label,dom('span',{id,class:'field-value'},display(field.value)));
  const isToggle=range.min===0&&range.max===1&&range.decimals===0;
  const onChange=()=>{apply.disabled=String(field.value)===input.value||!input.checkValidity();};
  const input=isToggle ? dom('select',{id,class:'field-input',onchange:onChange},
    dom('option',{value:'0',selected:field.value===0},'0'),dom('option',{value:'1',selected:field.value===1},'1')):
    dom('input',{id,class:'field-input',type:'number',value:field.value,min:range.min,max:range.max,step:1/(10**range.decimals),oninput:onChange,required:true});
  const apply=btn('Apply',async()=>{
    const value=input.value;
    if(!input.reportValidity()||!await confirmChange('Apply '+field.label+'?',display(field.value)+' → '+value,'Apply'))return;
    apply.disabled=true;
    try {await writeSetting(group,field.key,field.value,value);toast('Controller acknowledged the change.');reload();}
    catch(error){toast(error.message);onChange();}
  },'secondary',{disabled:true});
  return dom('div',{class:'field'},label,input,apply);
}
function writeSetting(group,key,current,value) {
  return api('settings/'+group+'/'+key,{method:'PUT',body:JSON.stringify({current:String(current),value:String(value)})});
}

function timerEditor(group,fields,canEdit,reload) {
  const byDay=Object.fromEntries(fields.map(field=>[field.key,field]));
  const schedule=days.map(day=>[...String(byDay[day]?.value??0).padStart(12,'0')]);
  const originals=schedule.map(periods=>periods.join(''));
  const table=dom('table',{class:'timer'},dom('thead',{},dom('tr',{},dom('th',{scope:'col'},'Controller time'),
    days.map(day=>dom('th',{scope:'col'},textLabel(day).slice(0,3))))));
  const rows=[];
  const showHour=hour=>String(hour).padStart(2,'0')+':00';
  for(let hour=0;hour<12;hour++){
    const slotName=showHour(hour*2)+'–'+showHour((hour+1)*2);
    rows.push(dom('tr',{},dom('th',{scope:'row'},slotName),days.map((day,index)=>{
      const maximum=byDay[day]?.range?.max;
      const modes=group==='boiler'?['On','Mode 1','Mode 2']:['On','Off'];
      if (!maximum||!canEdit())return dom('td',{},modes[Number(schedule[index][hour])]??'—');
      const select=dom('select',{class:'slot','aria-label':textLabel(day)+' '+slotName,
        onchange:event=>{schedule[index][hour]=event.target.value;apply.disabled=schedule.every((v,i)=>v.join('')===originals[i]);}},
        modes.map((name,value)=>dom('option',{value,selected:schedule[index][hour]===String(value)},name)));
      return dom('td',{},select);
    })));
  }
  table.append(dom('tbody',{},rows));
  const errorList=dom('p',{class:'muted','aria-live':'polite'});
  const apply=btn('Apply schedule',async()=>{
    if(!await confirmChange('Apply weekly schedule?','Changes are submitted to the controller one day at a time.','Apply schedule'))return;
    apply.disabled=true;
    table.querySelectorAll('select').forEach(item=>item.disabled=true);
    const errors=[];
    for(const [index,day] of days.entries()){
      const value=schedule[index].join('');
      if(value===originals[index])continue;
      try {await writeSetting(group,day,originals[index],value);originals[index]=value;}
      catch(error){errors.push(textLabel(day)+': '+error.message);}
    }
    if(errors.length){errorList.textContent=errors.join(' ');table.querySelectorAll('select').forEach(item=>item.disabled=false);apply.disabled=false;}
    else {toast('Controller acknowledged the schedule.');reload();}
  },'primary',{disabled:true});
  return card('Weekly schedule','Two-hour blocks use the controller clock. Enable the schedule separately in controller settings.',
    dom('div',{class:'table-scroll'},table),dom('div',{class:'toolbar'},canEdit()&&apply,errorList));
}

function helpPage() {
  const faqs=[
    ['Where do readings come from?','Boiler Mate reads one NBE controller directly over your local network. A controller can omit signals for optional hardware.'],
    ['Why is the interface read only?','The bridge needs a controller password and a separate web password to enable controls. If readings become stale, controls pause until the controller responds.'],
    ['How are charts and consumption stored?','Charts are recorded by this instance and require a persistent data directory. Pellet consumption periods come from the controller itself.'],
    ['How do schedules work?','Boiler and hot-water schedules are stored in the controller and follow its clock. Change the relevant schedule enable setting after reviewing the weekly blocks.'],
    ['What do controller statuses mean?','Statuses describe the current operating stage or an alarm. Open Messages for controller information and Events for the local activity log. Address the cause at the boiler before resetting an alarm.'],
    ['How can I change the dashboard?','Open Dashboard display on the overview to choose up to four readings. Choose signals and periods independently on Charts. Selections are stored in this browser.'],
    ['Where do I find help for a setting?','Open Help below a setting for its purpose and controller-reported range. Consult the manual for firmware-specific wiring and service procedures.'],
  ];
  main.replaceChildren(heading('Help & information','Operation, history, and troubleshooting for this Boiler Mate instance.'),
    dom('div',{class:'grid split'},
      card('Using Boiler Mate',null,faqs.map(([title,body])=>dom('details',{},dom('summary',{},title),dom('p',{},body)))),
      card('Manufacturer resources',null,
        dom('p',{},'Find installation and service procedures for your hardware in the controller manuals.'),
        dom('p',{},dom('a',{href:'https://www.nbe.dk/program-download/',target:'_blank',rel:'noreferrer'},'NBE controller resources')),
        dom('p',{},dom('a',{href:'https://github.com/mlipscombe/boiler-mate',target:'_blank',rel:'noreferrer'},'Boiler Mate project and configuration')))));
}

let pageAbort=new AbortController();
function renderPage() {
  pageAbort.abort();
  pageAbort=new AbortController();
  const requested=window.location.hash.slice(1);
  page=navigation.flatMap(([,items])=>items).some(([id])=>id===requested)?requested:'overview';
  const token=++generation;
  renderNavigation();
  if (groups[page]) settingsPage(page,token);
  else if (page==='overview') overview(token);
  else if (page==='charts') charts(token);
  else if (page==='consumption') consumption(token);
  else if (page==='events'||page==='messages') activity(token,page==='messages');
  else if (page==='notifications') notifications(token);
  else helpPage();
}

window.addEventListener('hashchange',()=>{if(window.location.hash!=='#content')renderPage();});
refresh().then(renderPage);
setInterval(refresh,5000);
setInterval(checkDesktopNotices,15000);
