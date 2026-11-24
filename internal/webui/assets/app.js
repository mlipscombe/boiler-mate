'use strict';

const navigation = [
  ['Monitor', [
    ['overview', 'Overview', '◈'], ['live-data', 'Live data', '◉'],
    ['charts', 'Charts', '∿'], ['consumption', 'Consumption', '▥'],
    ['events', 'Events', '☷'],
  ]],
  ['Controller', [
    ['boiler', 'Boiler', '▣'], ['fuel', 'Hopper & feed', '▧'], ['hot-water', 'DHW', '◉'],
    ['weather', 'Weather', '☁'], ['solar', 'Solar', '☼'], ['system', 'System', '⚙'],
  ]],
  ['Resources', [['help', 'Help & information', 'ⓘ']]],
];
const groups = {
  boiler: ['Boiler', 'Temperature, heating schedule, and external thermostat.', [
    ['boiler','Temperature & schedule','temperature-and-schedule'], ['regulation','Regulation'], ['oxygen','Oxygen'], ['cleaning','Cleaning'],
    ['fan','Fan'], ['ignition','Ignition'], ['pump','Pump'], ['alarm','Alarm'],
  ]],
  fuel: ['Hopper & feed', 'Fuel level, pellet feed, and auger calibration.', [
    ['hopper','Hopper'], ['auger','Augers & external feed'], ['vacuum','Vacuum transport'],
  ]],
  'hot-water': ['DHW', 'Domestic hot water temperature, output, and weekly schedule.', [['hot_water','Temperature & schedule','temperature-and-schedule']]],
  weather: ['Weather', 'Outdoor readings and weather compensation for both heating circuits.', [
    ['weather','Heating circuit 1','circuit-1'], ['weather2','Heating circuit 2','circuit-2'],
  ]],
  solar: ['Solar', 'Solar collector, cylinder temperatures, and pump settings.', [['sun','Solar circuit','solar-circuit']]],
  system: ['System', 'Controller information and service settings.', [['misc','Controller','controller'], ['manual','Service','service']]],
};
const readingGroups = ['operating_data','advanced_data'];
const readingAliases = {
  o2_corr_low:'corr_low',o2_corr_medium:'corr_medium',o2_corr_high:'corr_high',clouds:'cloud_level',
};
const combustionReadings = new Set(['oxygen','corr_low','corr_medium','corr_high','photo_level','smoke_temp']);
const consumptionPeriods = [['hours','Hourly'],['days','Daily'],['months','Monthly'],['years','Yearly']];
const labels = {
  boiler_temp: ['Boiler temperature','°C'], boiler_ref: ['Boiler target temperature','°C'],
  smoke_temp: ['Chimney/smoke','°C'], return_temp: ['Boiler return','°C'], shaft_temp: ['Drop shaft','°C'],
  dhw_temp: ['DHW temperature','°C'], dhw_ref: ['DHW target temperature','°C'], dhw_valve_state: ['DHW output',''],
  oxygen: ['Oxygen','%'], oxygen_ref: ['Oxygen target','%'],
  corr_low: ['O2 low regulation','%'], corr_medium: ['O2 mid regulation','%'], corr_high: ['O2 high regulation','%'],
  power_kw: ['Power output','kW'], power_pct: ['Power','%'], photo_level: ['Photo sensor (light)','lx'],
  fan_speed: ['Fan speed','%'], internet_uptime: ['Online time','%'], flow1: ['Flow','L/h'],
  content: ['Pellets remaining','kg'], distance: ['Distance to pellets','cm'], pressure: ['Air pressure','bar'],
  auger_capacity: ['Auger capacity','g'], kw_min: ['Power 10%','kW'], kw_max: ['Power 100%','kW'],
  consumption_total: ['Consumption total','kg'], trip1: ['Trip 1','kg'], trip2: ['Trip 2','kg'],
  feed_low: ['Auger low','%'], feed_medium: ['Auger mid','%'], feed_high: ['Auger high','%'],
  external_temp: ['External temp.','°C'], outdoor_temp: ['Outdoor weather','°C'],
  mean_out_temp: ['Average temperature','°C'], mean2_out_temp: ['Average temp. zone 2','°C'],
  chill_out: ['Chill','°C'], chill2_out: ['Chill zone 2','°C'],
  wind_speed: ['Wind speed','m/s'], wind_direction: ['Wind direction',''], air_pressure: ['Air pressure','hPa'],
  cloud_level: ['Clouds','%'], humidity: ['Humidity','%'],
  forward_temp: ['Zone 1 T6','°C'], forward2_temp: ['Zone 2','°C'],
  forward_ref: ['Zone 1 flow temperature target','°C'], forward2_ref: ['Zone 2 flow temperature target','°C'],
  sun_temp: ['Sun panel','°C'], sun2_temp: ['Sun panel 2','°C'], sun_dhw_temp: ['DHW bottom','°C'],
  sun_power_kw: ['Sun power','kW'], sun_pumpspeed: ['Sun pump (on/off)','%'],
  sun_pump_state: ['Sun pump state',''], sun_surplus_temp: ['Solar surplus temperature','°C'],
  t6_temp: ['T6 temperature','°C'], t7_temp: ['T7 temperature','°C'],
  total_consumption_24hours: ['Consumption last 24 h.','kg'], dhw_consumption_24hours: ['DHW consumption last 24 h.','kg'],
  dhw_part_24hours: ['Heat/DHW distribution 24h','%'], program_version: ['Firmware version',''],

  // The advanced view is flat, so qualify names that StokerCloud repeats in groups.
  boiler_setpoint: ['Wanted boiler temp.','°C'], boiler_diff: ['Boiler difference','°C'], boiler_diffsum: ['Boiler diff sum','°C'],
  boiler_gain_i: ['Power gain I',''], boiler_gain_p: ['Power gain P',''],
  boiler_part_i: ['Boiler power gain I','%'], boiler_part_p: ['Boiler power gain P','%'],
  boiler_power_actual: ['Actual boiler power','%'], boiler_power_kw: ['Boiler power','kW'], boiler_power_total: ['Total boiler power','%'],
  dhw_boiler_setpoint: ['Wanted DHW boiler temp.','°C'], dhw_diff: ['DHW difference','°C'], dhw_diffsum: ['DHW diff sum','°C'],
  dhw_gain_i: ['DHW gain I',''], dhw_gain_p: ['DHW gain P',''],
  dhw_part_i: ['DHW power gain I','%'], dhw_part_p: ['DHW power gain P','%'],
  dhw_power_actual: ['Actual DHW power','%'], dhw_power_kw: ['DHW power','kW'], dhw_power_total: ['Total DHW power','%'],
  o2_setpoint: ['Wanted O2','%'], o2_diff: ['O2 difference','%'], o2_diffsum: ['O2 diff sum','%'],
  o2_gain_i: ['O2 gain I',''], o2_gain_p: ['O2 gain P',''],
  o2_gain_pellets: ['Pellet gain P',''], o2_gain_pellets_i: ['Pellet gain I',''],
  o2_part_i: ['Air change gain I','%'], o2_part_p: ['Air change gain P','%'],
  o2_power_actual: ['Actual O2','%'], o2_power_total: ['Total O2 change','%'], o2_power_max: ['Max. O2 change','%'],
  auger_run: ['Auger runtime','s'], auger_pause: ['Auger pause time','s'],
};
const labelCapitalization = {dhw:'DHW',o2:'O2',kw:'kW',i:'I',p:'P',rpm:'RPM',id:'ID',dl:'DL'};
const colors = ['#345ece','#118576','#b25e35','#a65095','#5c7e36','#4685ad','#ab7f2e','#64748b'];
const days = ['monday','tuesday','wednesday','thursday','friday','saturday','sunday'];
const datetime = new Intl.DateTimeFormat(undefined,{dateStyle:'medium',timeStyle:'short'});
const shorttime = new Intl.DateTimeFormat(undefined,{hour:'numeric',minute:'2-digit'});
const numeric = new Intl.NumberFormat(undefined,{maximumFractionDigits:2});
const main = document.querySelector('main');
let status = {data:{},online:false,control:false};
let page = '';
let generation = 0;
let statusRequest = 0;
let statusSource = null;
let statusPoll = 0;
let refreshPageHistory = null;
let refreshPageEvents = null;
let resizeChart = null;
let toastTimer = 0;
let noticeReady = false;
let lastDesktopNotice = 0;
let noticeFilter = saved('noticeFilter', ['alarm','info','connection']);
let chosenSignals = saved('signals',['operating_data.boiler_temp','operating_data.boiler_ref']);
let chosenMetrics = saved('metrics',['operating_data.external_temp','operating_data.air_flow'])
  .filter(key=>!combustionReadings.has(readingName(key)));

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
  const label=labels[key]?.[0];
  if (label) return label;
  return key.split('_').map((word,index)=>{
    if (Object.hasOwn(labelCapitalization,word)) return labelCapitalization[word];
    return index===0?word.charAt(0).toUpperCase()+word.slice(1):word;
  }).join(' ');
}
function display(value) {
  if (value == null || value === '') return '—';
  return typeof value === 'number' ? numeric.format(value) : String(value);
}
function snapshot(group) { return status.data[group]?.values ?? {}; }
function signal(key) {
  const [group,name] = key.split('.');
  const value = snapshot(group)[name];
  if (unitSignal(key) === '°C' && typeof value === 'number' && value >= 999) return null;
  return value;
}
function readingName(key) {
  const name = key.split('.').at(-1);
  return Object.hasOwn(readingAliases,name) ? readingAliases[name] : name;
}
function titleSignal(key) { return textLabel(readingName(key)); }
function unitSignal(key) { return labels[readingName(key)]?.[1] ?? ''; }
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
function renderSections(pageID,sections,active,label) {
  return dom('nav',{class:'switcher','aria-label':label},sections.map(([id,name,slug=id])=>
    dom('a',{href:'#'+pageID+'/'+slug,class:'secondary','aria-current':id===active?'page':null},name)));
}
function renderNavigation(sectionLabel) {
  document.getElementById('navigation').replaceChildren(...navigation.map(([name,links]) =>
    dom('div',{class:'nav-group'},dom('div',{class:'nav-label'},name),
      links.map(([id,label])=>dom('a',{href:'#'+id,class:'side-link','aria-current':id===page?'page':null},label)))));
  const mobile = document.getElementById('mobile-navigation');
  mobile.replaceChildren(...navigation.map(([name,links]) =>
    dom('optgroup',{label:name},links.map(([id,label])=>dom('option',{value:id,selected:id===page},label)))));
  mobile.onchange = () => { window.location.hash = '#'+mobile.value; };
  const label = navigation.flatMap(([,links])=>links).find(([id])=>id===page)?.[1] ?? 'Overview';
  document.getElementById('breadcrumb').textContent = [label,sectionLabel].filter(Boolean).join(' / ');
  document.title = [sectionLabel,label,'Boiler Mate'].filter(Boolean).join(' · ');
}
function refreshChrome() {
  document.getElementById('sidebar-indicator').classList.toggle('live',Boolean(status.online));
  document.getElementById('sidebar-status').textContent = status.online ? 'Controller connected' : 'Controller unavailable';
  document.getElementById('serial').textContent = status.serial ? 'NBE '+status.serial : '';
  const access = document.getElementById('access');
  access.textContent = status.control ? 'Controls enabled' : 'Read only';
  access.className = 'pill '+(status.control?'good':'warn');
}
function applyStatus(next, resuming = false) {
  const previous = status;
  status = next;
  refreshChrome();
  if (page === 'overview') updateOverview();
  if (page==='live-data' && readingGroups.some(group=>previous.data[group]?.at!==status.data[group]?.at)) updateReadings();
  for (const reading of main.querySelectorAll('[data-live-metric]')) {
    reading.textContent = display(signal(reading.dataset.liveMetric));
  }
  if (status.historyRevision!==previous.historyRevision) refreshPageHistory?.();
  if (resuming || status.eventRevision!==previous.eventRevision) refreshPageEvents?.();
}
async function refresh(resuming = false) {
  const request = ++statusRequest;
  let next;
  try { next = await api('status'); }
  catch {
    next = {...status,online:false,control:false};
    resuming = false;
  }
  if (statusRequest===request) applyStatus(next,resuming);
}
function stopStatusPolling() {
  clearInterval(statusPoll);
  statusPoll = 0;
}
function startStatusPolling() {
  if (statusPoll || document.hidden) return;
  refresh(true);
  statusPoll = setInterval(refresh,5000);
}
function stopStatusStream() {
  statusSource?.close();
  statusSource = null;
  stopStatusPolling();
  statusRequest++;
}
function connectWhileVisible() {
  if (document.hidden) {
    stopStatusStream();
    return;
  }
  if (statusSource || statusPoll) return;
  if (typeof EventSource !== 'function') {
    startStatusPolling();
    return;
  }
  const source = new EventSource('/api/status/stream');
  statusSource = source;
  let resuming = true;
  source.onopen = ()=>{resuming=true;};
  source.onmessage = event=>{
    if (statusSource!==source) return;
    let next;
    try { next = JSON.parse(event.data); }
    catch { startStatusPolling(); return; }
    statusRequest++;
    stopStatusPolling();
    applyStatus(next,resuming);
    resuming=false;
  };
  source.onerror = ()=>{ if (statusSource===source) startStatusPolling(); };
}
function metric(key, sub='') {
  return dom('article',{class:'card metric'},dom('p',{class:'metric-label'},titleSignal(key)),
    dom('div',{class:'metric-reading'},dom('span',{'data-live-metric':key},display(signal(key))),unitSignal(key)&&dom('small',{},unitSignal(key))),
    sub&&dom('p',{class:'metric-sub'},sub));
}
function updateOverview() {
  const live = document.getElementById('overview-live');
  if (!live) return;
  const op = snapshot('operating_data');
  const blocked = !status.online ? 'Waiting for a live controller reading.'
    : !status.control ? 'Controller writes are disabled.'
    : !['ON','OFF'].includes(op.state_on) ? 'Waiting for the boiler state.' : '';
  const command = action => async ()=>{
    const label = action==='start'?'Start boiler':'Stop boiler';
    const prompt = action==='start'?'Send a start request to the pellet boiler.':'Send a stop request to the pellet boiler.';
    if (!await confirmChange(label+'?',prompt,label)) return;
    try { await api('power',{method:'POST',body:JSON.stringify({action})}); toast(label+' request acknowledged.'); await refresh(); }
    catch(error) { toast(error.message); }
  };
  const last = status.data.operating_data?.at;
  const focusedAction = live.contains(document.activeElement) ? document.activeElement.dataset.action : null;
  const remaining = signal('operating_data.content');
  const output = signal('operating_data.power_kw');
  const water = signal('operating_data.dhw_temp');
  const returnTemp = signal('operating_data.return_temp');
  const readings = [
    ['Fuel hopper',remaining!=null&&remaining<0 ? 'Below minimum' : withUnit(remaining,'kg'),withUnit(signal('advanced_data.total_consumption_24hours'),'kg')+' used in 24 h','#fuel'],
    ['Boiler',withUnit(output,'kW'),withUnit(signal('operating_data.power_pct'),'%')+' burner output','#boiler'],
    ['Heating circuit',withUnit(returnTemp,'°C'),'Return water','#weather'],
    ['DHW',water==null?'No reading':withUnit(water,'°C'),'Cylinder sensor','#hot-water'],
  ];
  live.replaceChildren(dom('section',{class:'overview-system','aria-label':'Current boiler state'},
    dom('div',{class:'system-main'},
      dom('div',{},dom('div',{class:'operating-mode'},dom('span',{class:'indicator'+(status.online?' live':'')}),status.online?'Controller online':'Controller offline'),
        dom('h2',{},op.state_text || 'Waiting for data'),
        last&&dom('p',{class:'muted'},'Updated '+shorttime.format(new Date(last)))),
      dom('div',{class:'system-temperature'},dom('span',{},'Boiler temperature'),
        dom('strong',{},display(signal('operating_data.boiler_temp')),dom('small',{},'°C')),
        dom('span',{},'Target '+withUnit(signal('operating_data.boiler_ref'),'°C'))),
      dom('div',{class:'power-controls','aria-label':'Boiler controls'},
        dom('h3',{},'Boiler controls'),
        dom('div',{class:'power-buttons'},
          btn('Start boiler',command('start'),'primary',{'data-action':'start',disabled:Boolean(blocked)||op.state_on!=='OFF'}),
          btn('Stop boiler',command('stop'),'danger',{'data-action':'stop',disabled:Boolean(blocked)||op.state_on!=='ON'})),
        blocked&&dom('p',{},blocked))),
    dom('div',{class:'system-line'},readings.map(([label,value,detail,link]) =>
      dom('a',{class:'system-reading',href:link},dom('span',{},label),dom('strong',{},value),dom('small',{},detail))))));
  const combustion = document.getElementById('overview-combustion');
  if (combustion) renderCombustion(combustion);
  const custom = document.getElementById('overview-custom');
  if (custom) custom.replaceChildren(...chosenMetrics.map(key=>{
    const value=signal(key);
    return dom('div',{class:'compact-reading'},dom('span',{},titleSignal(key)),
      dom('strong',{},withUnit(value,unitSignal(key))));
  }));
  if (focusedAction) live.querySelector('[data-action="'+focusedAction+'"]')?.focus({preventScroll:true});
}
function withUnit(value,unit) {
  return display(value)+(value==null||!unit?'':' '+unit);
}
function renderCombustion(target) {
  const reading = (name,key,emphasis=false)=>{
    const value = status.online ? signal('operating_data.'+key) : null;
    return dom('div',{class:'combustion-reading'+(emphasis?' combustion-oxygen':'')},
      dom('span',{class:'metric-label'},name),
      dom('strong',{class:'combustion-value'},display(value),value!=null&&dom('small',{},unitSignal(key))));
  };
  const compensation = [['Low','corr_low'],['Medium','corr_medium'],['High','corr_high']];
  target.replaceChildren(
    reading('Oxygen','oxygen',true),
    reading('Photo sensor','photo_level'),
    reading('Chimney smoke','smoke_temp'),
    dom('div',{class:'combustion-reading combustion-compensation'},dom('span',{class:'metric-label'},'O2 compensation'),
      dom('dl',{},compensation.map(([name,key])=>{
        const value = status.online ? signal('operating_data.'+key) ?? signal('advanced_data.o2_'+key) : null;
        return dom('div',{},dom('dt',{},name),dom('dd',{},withUnit(value,'%')));
      }))));
}
function overview(token) {
  const live = dom('div',{id:'overview-live'});
  const chartBody = dom('div',{},empty('Loading temperature history…'));
  const recent = dom('div',{},empty('Loading recent events…'));
  const metricsPanel = dom('div',{},empty('Waiting for telemetry…'));
  const customise = dom('details',{},dom('summary',{},'Choose overview readings'),metricsPanel);
  main.replaceChildren(heading('Pellet boiler'),
    dom('div',{class:'stack'},live,
      card('Combustion',null,dom('div',{class:'combustion-grid',id:'overview-combustion'})),
      dom('div',{class:'grid split'},
        card('Temperature · 24 hours',null,chartBody,dom('a',{class:'home-link',href:'#charts'},'All charts')),
        card('Recent events',null,recent,dom('a',{class:'home-link',href:'#events'},'View all events'))),
      card('Other readings',null,dom('div',{class:'compact-readings',id:'overview-custom'}),customise)));
  updateOverview();
  const renderChooser = ()=>chooseSignals(metricsPanel,chosenMetrics,selection=>{
    chosenMetrics=selection;
    saveLocal('metrics',selection);
    updateOverview();
    renderChooser();
  },{exclude:combustionReadings});
  customise.addEventListener('toggle',()=>{if(customise.open)renderChooser();});
  let historyRequest = 0;
  const refreshChart = async(showError=false)=>{
    const request = ++historyRequest;
    try {
      const data = await loadHistory(['operating_data.boiler_temp','operating_data.boiler_ref'],86400);
      if(token===generation&&request===historyRequest) showChart(chartBody,data,86400,true);
    } catch(error) {
      if(showError&&token===generation&&request===historyRequest) chartBody.replaceChildren(empty(error.message));
    }
  };
  refreshPageHistory = refreshChart;
  refreshChart(true);
  let recentRequest = 0;
  let recentLoaded = false;
  const refreshRecent = async()=>{
    const request = ++recentRequest;
    try {
      const items = await api('events');
      if(token!==generation||request!==recentRequest)return;
      recent.replaceChildren(eventList(items.slice(0,4)));
      recentLoaded = true;
    } catch(error) {
      if(!recentLoaded&&token===generation&&request===recentRequest)recent.replaceChildren(empty(error.message));
    }
  };
  refreshPageEvents = refreshRecent;
  refreshRecent();
}

function liveReadingKeys() {
  const keys = new Map();
  for (const group of readingGroups) {
    for (const name of Object.keys(snapshot(group))) {
      const key = group+'.'+name;
      const canonical = readingName(name);
      if (!keys.has(canonical) || signal(keys.get(canonical))==null) keys.set(canonical,key);
    }
  }
  return [...keys.values()];
}
function measurementKeys(selected) {
  const keys = new Set(selected);
  const selectedNames = new Set(selected.map(readingName));
  for (const key of liveReadingKeys()) {
    const name = readingName(key);
    if (!selectedNames.has(name) && typeof signal(key)==='number' && !['time','state','substate'].includes(name)) keys.add(key);
  }
  return [...keys].sort((a,b)=>titleSignal(a).localeCompare(titleSignal(b))||a.localeCompare(b));
}
function chooseSignals(target,selected,change,{limit=Infinity,exclude}={}) {
  const keys = measurementKeys(selected).filter(key=>!exclude?.has(readingName(key)));
  const previous = target.querySelector('.signal-chooser');
  const focused = previous?.contains(document.activeElement) ? document.activeElement.value : null;
  const scrollTop = previous?.scrollTop ?? 0;
  const chooser = dom('div',{class:'signal-chooser'},keys.map(key=>{
    const checked = selected.includes(key);
    return dom('label',{},dom('input',{type:'checkbox',value:key,checked,
      disabled:!checked&&selected.length>=limit,
      onchange:()=>change(checked?selected.filter(k=>k!==key):[...selected,key])}),titleSignal(key));
  }));
  target.replaceChildren(keys.length ? chooser : empty('Measurements will appear after the first controller reading.'));
  chooser.scrollTop = scrollTop;
  if (focused) {
    [...chooser.querySelectorAll('input')].find(input=>input.value===focused)?.focus({preventScroll:true});
  }
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
  const load = async(showLoading=true)=>{
    const id = ++request;
    if (!chosenSignals.length) {
      chartBody.replaceChildren(empty('Choose up to eight signals to chart.'));
      csv.disabled=true; return;
    }
    if(showLoading) {
      csv.disabled=true;
      chartBody.replaceChildren(empty('Loading history…'));
    }
    try {
      const loaded = await loadHistory(chosenSignals,duration);
      if(token!==generation||request!==id)return;
      data=loaded;
      showChart(chartBody,data,duration);
      csv.disabled=!Object.values(data).some(points=>points.length);
    } catch(error) {
      if(request===id&&token===generation&&showLoading)chartBody.replaceChildren(empty(error.message));
    }
  };
  const choose = ()=>chooseSignals(signals,chosenSignals,selection=>{
    chosenSignals=selection; saveLocal('signals',selection); choose(); load();
  },{limit:8});
  main.replaceChildren(heading('Charts','Compare live signals recorded by Boiler Mate. History starts when this instance is enabled.'),
    card('Signal history','Each signal keeps its own unit.',
      dom('div',{class:'toolbar'},dom('label',{},'Period',range),csv),
      chartBody,dom('details',{open:true},dom('summary',{},'Select signals'),signals)));
  refreshPageHistory = ()=>load(false);
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
  resizeChart = null;
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
  resizeChart = render;
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

function consumption(period,token) {
  const output=dom('div',{},empty('Reading consumption from the controller…'));
  const selector=renderSections('consumption',consumptionPeriods,period,'Consumption period');
  const load=async()=>{
    output.replaceChildren(empty('Reading consumption from the controller…'));
    try {
      const data=await api('consumption?period='+period);
      if(token!==generation)return;
      const aggregate=data.total.reduce((sum,value)=>sum+value,0);
      const dhw=data.hotWater?.reduce((sum,value)=>sum+value,0);
      output.replaceChildren(dom('div',{class:'grid metrics'},
        stat('Period total',aggregate,'kg'),stat('DHW',dhw,'kg'),stat('Controller counter',data.counter?.[0],'kg')),
        dom('div',{class:'table-scroll'},consumptionTable(data)));
    } catch(error) {if(token===generation)output.replaceChildren(empty(error.message),btn('Try again',load));}
  };
  main.replaceChildren(heading('Consumption','Historical pellet use stored in the controller, including the domestic hot-water share when available.'),
    card('Pellet use',null,selector,output));
  load();
}
function stat(label,value,unit) {
  return dom('article',{class:'card metric'},dom('p',{class:'metric-label'},label),
    dom('p',{class:'metric-reading'},display(value),unit&&dom('small',{},unit)));
}
function consumptionTable(data) {
  const max = Math.max(...data.total,1);
  return dom('table',{},dom('thead',{},dom('tr',{},
    ['Period','Total (kg)','DHW (kg)','Heating (kg)'].map(name=>dom('th',{scope:'col'},name)))),
    dom('tbody',{},data.total.map((value,i)=>{
      const hot=data.hotWater?.[i];
      const heat=hot==null ? null : Math.max(0,value-hot);
      return dom('tr',{},dom('th',{scope:'row'},(i+1)+' / '+data.total.length),
        dom('td',{},dom('meter',{value,min:0,max,'aria-label':'Total pellet use'}),' '+display(value)),
        dom('td',{},display(hot)),dom('td',{},display(heat)));
    })));
}

function eventList(rows,onRead) {
  if(!rows.length)return empty('No events to show.');
  return dom('ul',{class:'notice-list'},rows.map(row=>dom('li',{},
    pill(textLabel(row.kind),row.kind==='alarm'?'error':row.kind==='connection'?'warn':''),
    dom('div',{},dom('strong',{},row.message),dom('small',{},datetime.format(new Date(row.time*1000)))),
    onRead&&!row.read&&row.kind!=='state'&&btn('Mark read',async event=>{
      const button=event.currentTarget;
      button.disabled=true;
      try {
        await api('notifications/'+row.id+'/read',{method:'POST'});
        onRead(row.id);
      } catch(error) {
        button.disabled=false;
        toast(error.message);
      }
    },'subtle'))));
}
function activity(token,initialFilter,notificationsOpen) {
  const entries=dom('div',{});
  const filter=dom('select',{'aria-label':'Filter events'},
    [['','All events'],['notices','Notices'],['unread','Unread notices'],['alarm','Alarms'],['info','Information'],
      ['connection','Connection'],['state','State changes'],['command','Commands']]
      .map(([key,label])=>dom('option',{value:key,selected:key===initialFilter},label)));
  const more=btn('Load older',()=>load('older'));
  more.hidden=true;
  const seen=new Set();
  let rows=[];
  let request=0;
  let loading=false;
  let refreshPending=false;
  let initialized=false;
  const markRead=id=>{
    if(token!==generation)return;
    seen.add(id);
    const row=rows.find(item=>item.id===id);
    if(row&&!row.read){
      row.read=true;
      render();
    }
  };
  const render=()=>{
    const shown=filter.value==='unread'?rows.filter(row=>!row.read):rows;
    entries.replaceChildren(eventList(shown,markRead));
  };
  const refreshEvents=()=>{
    if(document.hidden)return;
    if(loading)refreshPending=true;
    else load('refresh');
  };
  const load=async mode=>{
    const older=mode==='older';
    const refreshing=mode==='refresh';
    const id=++request;
    const current=()=>token===generation&&id===request;
    const args=new URLSearchParams();
    if(filter.value)args.set('filter',filter.value);
    if(older)args.set('before',rows.at(-1).id);
    const through=refreshing?rows.at(-1)?.id:undefined;
    if(mode==='reset'){
      rows=[];
      initialized=false;
      more.hidden=true;
      entries.replaceChildren(empty('Loading events…'));
    }
    loading=true;
    more.disabled=true;
    try {
      const result=[];
      let hasOlder;
      while(true){
        const batch=await api('events?'+args);
        if(!current()||(refreshing&&document.hidden))return;
        // Keep the already loaded history without pulling in an extra page of older entries.
        const pageRows=result.length&&refreshing?batch.filter(row=>row.id>=through):batch;
        result.push(...pageRows);
        hasOlder=batch.length===150||pageRows.length<batch.length;
        const oldest=batch.at(-1)?.id;
        if(!hasOlder||!(oldest>through))break;
        args.set('before',oldest);
      }
      const updated=(older?[...rows,...result]:result).filter(row=>{
        if(seen.has(row.id))row.read=true;
        return filter.value!=='unread'||!row.read;
      });
      const changed=!initialized||updated.length!==rows.length||
        updated.some((row,index)=>row.id!==rows[index].id||row.read!==rows[index].read);
      rows=updated;
      initialized=true;
      more.hidden=!hasOlder||!rows.length;
      if(changed)render();
    } catch(error) {
      if(!current())return;
      if(older)toast(error.message);
      else if(!initialized)entries.replaceChildren(empty(error.message),btn('Try again',()=>load('reset')));
    } finally {
      if(current()){
        loading=false;
        more.disabled=false;
        if(refreshPending){
          refreshPending=false;
          refreshEvents();
        }
      }
    }
  };
  filter.addEventListener('change',()=>load('reset'));
  main.replaceChildren(heading('Events'),dom('div',{class:'stack'},
    dom('section',{class:'card'},desktopNotifications(notificationsOpen)),
    dom('section',{class:'card'},dom('div',{class:'toolbar'},filter),entries,more)));
  refreshPageEvents=refreshEvents;
  load('reset');
}
function desktopNotifications(open) {
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
  return dom('details',{open},dom('summary',{},'Desktop notifications'),
    dom('p',{},'Choose which notices this browser can show while Boiler Mate is open.'),inputs,desktop);
}
async function checkDesktopNotices() {
  if(typeof Notification==='undefined'||Notification.permission!=='granted')return;
  try {
    const rows=await api('events?filter=notices');
    if(noticeReady)rows.filter(row=>row.id>lastDesktopNotice&&noticeFilter.includes(row.kind)&&!row.read)
      .reverse().forEach(row=>new Notification('Boiler Mate', {body:row.message,icon:'/icon-192.png',tag:'boiler-'+row.id}));
    lastDesktopNotice=Math.max(lastDesktopNotice,rows[0]?.id??0);
    noticeReady=true;
  } catch {}
}
function dataTable(query) {
  const entries=liveReadingKeys().sort((a,b)=>titleSignal(a).localeCompare(titleSignal(b)));
  if(!entries.length)return empty('The controller has not returned these readings.');
  const results=entries.filter(key=>[key,titleSignal(key)].some(label=>label.toLowerCase().includes(query)));
  if(!results.length)return empty('No readings match.');
  return dom('div',{class:'table-scroll'},dom('table',{},
    dom('thead',{},dom('tr',{},dom('th',{scope:'col'},'Signal'),dom('th',{scope:'col'},'Reading'))),
    dom('tbody',{},results.map(key=>dom('tr',{},
      dom('th',{scope:'row'},titleSignal(key)),dom('td',{},withUnit(signal(key),unitSignal(key))))))));
}
function updateReadings() {
  const filter=document.getElementById('readings-filter');
  if(!filter)return;
  const timestamp=Math.max(...readingGroups.map(group=>Date.parse(status.data[group]?.at)||0));
  document.getElementById('reading-time').textContent=timestamp?'Last sample '+shorttime.format(new Date(timestamp)):'';
  document.getElementById('reading-table').replaceChildren(dataTable(filter.value.trim().toLowerCase()));
}
function readingsPage() {
  const filter=dom('input',{id:'readings-filter',type:'search',class:'field-input lookup',
    'aria-label':'Filter readings',placeholder:'Filter readings',oninput:updateReadings});
  main.replaceChildren(heading('Live data'),dom('section',{class:'card'},
    dom('div',{class:'toolbar'},filter,dom('span',{id:'reading-time'})),dom('div',{id:'reading-table'})));
  updateReadings();
}
function settingsPage(id,group,token) {
  const [title,description,tabs]=groups[id];
  const controls=dom('div',{},empty('Loading settings…'));
  const tablist=renderSections(id,tabs,group,title+' settings categories');
  let sequence=0;
  const editEnabled=()=>status.control&&status.online;
  const metrics={
    boiler:['operating_data.boiler_temp','operating_data.return_temp','operating_data.power_kw','operating_data.smoke_temp'],
    fuel:['operating_data.content','operating_data.distance','advanced_data.total_consumption_24hours','advanced_data.dhw_consumption_24hours'],
    'hot-water':['operating_data.dhw_temp','operating_data.dhw_ref'],
    weather:['advanced_data.outdoor_temp','advanced_data.wind_speed','advanced_data.humidity','operating_data.forward_temp'],
    solar:['operating_data.sun_temp','operating_data.sun2_temp','operating_data.sun_dhw_temp','operating_data.sun_power_kw'],
    system:['advanced_data.program_version','operating_data.internet_uptime'],
  };
  const load=async()=>{
    const index=++sequence;
    controls.replaceChildren(empty('Loading controller values…'));
    try {
      const fields=await api('settings/'+group);
      if(token!==generation||index!==sequence)return;
      const inputs=dom('div',{class:'fields'});
      const options=fields.filter(field=>!field.timer);
      const update=query=>{
        const match=query.trim().toLowerCase();
        const filtered=options.filter(field=>(field.label+' '+field.key).toLowerCase().includes(match));
        inputs.replaceChildren(...(filtered.length?filtered.map(field=>settingRow(group,field,editEnabled,load)):[empty('No settings match.')]));
      };
      const search=dom('input',{type:'search',class:'field-input lookup','aria-label':'Filter settings',placeholder:'Filter settings',oninput:event=>update(event.target.value)});
      const timerFields=fields.filter(field=>field.timer);
      controls.replaceChildren(dom('div',{class:'toolbar'},search),inputs,
        timerFields.length&&timerEditor(group,timerFields,editEnabled,load));
      update('');
    } catch(error) {
      if(token===generation&&index===sequence)controls.replaceChildren(empty(error.message),btn('Try again',load));
    }
  };
  const actions=dom('div',{class:'toolbar page-actions'},
    id==='system'&&btn('Reset alarm',async()=>{
      if(!await confirmChange('Reset controller alarm?','Resolve the cause of the alarm before resetting it.','Reset alarm'))return;
      try {await api('alarm/reset',{method:'POST'});toast('Controller acknowledged the alarm reset.');}
      catch(error){toast(error.message);}
    },'danger',{disabled:!editEnabled()}),
    id==='boiler'&&dom('a',{class:'primary',href:'#overview'},'Boiler controls'),
    btn('Refresh readings',async()=>{await refresh();renderPage();}));
  main.replaceChildren(heading(title,description,actions),
    dom('div',{class:'stack'},
      dom('div',{class:'grid metrics'},metrics[id].map(key=>metric(key))),
      card('Controller settings',status.control?'Changes are sent to the controller after confirmation.':'You can view settings in read-only mode.',tablist,controls)));
  load();
}

function settingRow(group,field,canEdit,reload) {
  const range=field.range;
  const id='field-'+group+'-'+field.key;
  const key=group+'.'+field.key;
  const options=field.options ?? [];
  const currentValue=String(field.value);
  const optionLabel=value=>options.find(option=>option.value===String(value))?.label ?? display(value);
  const help=field.help||range||field.fixed ? dom('details',{class:'field-help',name:'setting-help'},
    dom('summary',{'aria-label':'Info about '+field.label},'i'),
    dom('div',{class:'field-help-content'},
      field.help&&dom('p',{},field.help),
      field.fixed&&dom('p',{},'This controller reports a fixed value with no editable range.'),
      range&&dom('p',{},options.length?'Default '+optionLabel(range.default):
        'Range '+display(range.min)+' to '+display(range.max)+' · Default '+display(range.default)),
      dom('code',{class:'field-id'},key))) : null;
  const label=dom('div',{class:'field-label'},dom('label',{for:id,class:'field-name',title:key},field.label),help);
  if(!range||!canEdit())return dom('div',{class:'field'},label,
    dom('div',{class:'field-value'},dom('span',{id},optionLabel(field.value)),field.fixed&&pill('Read only')));
  const onChange=()=>{apply.disabled=currentValue===input.value||!input.checkValidity();};
  const input=options.length ? dom('select',{id,class:'field-input',onchange:onChange},
    !options.some(option=>option.value===currentValue)&&dom('option',{value:currentValue,selected:true,disabled:true},'Current value ('+display(field.value)+')'),
    options.map(option=>dom('option',{value:option.value,selected:option.value===currentValue,disabled:option.disabled},option.label))):
    dom('input',{id,class:'field-input',type:'number',value:field.value,min:range.min,max:range.max,step:1/(10**range.decimals),oninput:onChange,required:true});
  const action=key==='auger.forced_run'?'Run auger':'Apply';
  const apply=btn(action,async()=>{
    const value=input.value;
    const proposed=options.length?optionLabel(value):value;
    if(!input.reportValidity()||!await confirmChange(action==='Run auger'?'Run external auger?':'Apply '+field.label+'?',optionLabel(field.value)+' → '+proposed,action))return;
    apply.disabled=true;
    try {await writeSetting(group,field.key,field.value,value);toast('Controller acknowledged the change.');reload();}
    catch(error){toast(error.message);onChange();}
  },'secondary field-action',{disabled:true});
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
      const modes=group==='boiler'?['On','Off','Night reduction']:['On','Off'];
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
    ['Why is the interface read only?','Writes require BOILER_MATE_ENABLE_WRITES=true and a controller PIN. Controls pause when readings become stale.'],
    ['How are charts and consumption stored?','Charts are recorded by this instance and require a persistent data directory. Pellet consumption periods come from the controller itself.'],
    ['How do schedules work?','Boiler and hot-water schedules are stored in the controller and follow its clock. Change the relevant schedule enable setting after reviewing the weekly blocks.'],
    ['What do controller statuses mean?','Statuses describe the current operating stage or an alarm. Events includes controller notices, the activity log, and desktop notification settings. Address the cause at the boiler before resetting an alarm.'],
    ['How can I change the dashboard?','Open Choose overview readings on Overview to select additional readings. Choose signals and periods independently on Charts. Selections are stored in this browser.'],
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

function renderPage() {
  refreshPageHistory = null;
  refreshPageEvents = null;
  resizeChart = null;
  const [hashPage,requestedSection]=window.location.hash.slice(1).split('/');
  const requested=({ 'advanced-data':'live-data', messages:'events', notifications:'events' })[hashPage]??hashPage;
  page=navigation.flatMap(([,items])=>items).some(([id])=>id===requested)?requested:'overview';
  const sections=groups[page]?.[2] ?? (page==='consumption'?consumptionPeriods:[]);
  const [section,sectionLabel]=sections.find(([id,,slug=id])=>slug===requestedSection) ?? sections[0] ?? [];
  const token=++generation;
  renderNavigation(requestedSection?sectionLabel:null);
  if (groups[page]) settingsPage(page,section,token);
  else if (page==='live-data') readingsPage();
  else if (page==='overview') overview(token);
  else if (page==='charts') charts(token);
  else if (page==='consumption') consumption(section,token);
  else if (page==='events') activity(token,hashPage==='messages'?'notices':'',hashPage==='notifications'||requestedSection==='notifications');
  else helpPage();
}

document.querySelector('.skip').addEventListener('click',event=>{event.preventDefault();main.focus();});
window.addEventListener('hashchange',()=>{if(window.location.hash!=='#content')renderPage();});
window.addEventListener('resize',()=>resizeChart?.());
document.addEventListener('visibilitychange',connectWhileVisible);
window.addEventListener('pagehide',stopStatusStream);
window.addEventListener('pageshow',connectWhileVisible);
refresh().then(()=>{renderPage();connectWhileVisible();});
setInterval(checkDesktopNotices,15000);
