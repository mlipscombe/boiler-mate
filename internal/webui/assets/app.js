'use strict';

const navigation = [
  ['Monitor', [
    ['overview', 'Overview', '◈'], ['live-data', 'Live data', '◉'],
    ['charts', 'Charts', '∿'], ['consumption', 'Consumption', '▥'],
    ['efficiency', 'Efficiency', '↗'],
    ['events', 'Events', '☷'],
  ]],
  ['Controller', [
    ['boiler', 'Boiler', '▣'], ['fuel', 'Hopper & feed', '▧'], ['hot-water', 'DHW', '◉'],
    ['weather', 'Weather', '☁'], ['solar', 'Solar', '☼'], ['system', 'System', '⚙'],
    ['external-heating', 'External heating', '◉'],
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
const readingGroups = ['operating_data','advanced_data','home_assistant'];
const readingAliases = {
  o2_corr_low:'corr_low',o2_corr_medium:'corr_medium',o2_corr_high:'corr_high',clouds:'cloud_level',
};
const combustionReadings = new Set(['oxygen','oxygen_ref','o2_setpoint','corr_low','corr_medium','corr_high','photo_level','smoke_temp']);
const consumptionPeriods = [['hours','Hourly'],['days','Daily'],['months','Monthly'],['years','Yearly']];
const consumptionSections = [...consumptionPeriods,['ledger','Purchase ledger']];
const labels = {
  boiler_temp: ['Boiler temperature','°C'], boiler_ref: ['Boiler target temperature','°C'],
  smoke_temp: ['Chimney/smoke','°C'], return_temp: ['Boiler return','°C'], shaft_temp: ['Drop shaft','°C'],
  dhw_temp: ['DHW temperature','°C'], dhw_ref: ['DHW target temperature','°C'], dhw_valve_state: ['DHW output',''],
  oxygen: ['Oxygen','%'], oxygen_ref: ['Oxygen target','%'],
  corr_low: ['O2 low regulation','%'], corr_medium: ['O2 mid regulation','%'], corr_high: ['O2 high regulation','%'],
  power_kw: ['Power output','kW'], power_pct: ['Power','%'], photo_level: ['Photo sensor (light)','%'],
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
const decimalTenths = new Intl.NumberFormat(undefined,{minimumFractionDigits:1,maximumFractionDigits:1});
const main = document.querySelector('main');
const chartRenderers = new Map();
let status = {data:{},online:false,control:false};
let page = '';
let generation = 0;
let statusRequest = 0;
let statusSource = null;
let statusPoll = 0;
let refreshPageHistory = null;
let refreshPageEvents = null;
let overviewPhaseTimer = 0;
let toastTimer = 0;
let noticeSource = null;
let noticePoll = 0;
let lastDesktopNotice = null;
let pushSetup = null;
let pushSubscription = null;
let noticeFilter = saved('noticeFilter', ['alarm','info','connection','fuel']);
let chosenSignals = saved('signals',['operating_data.boiler_temp','operating_data.boiler_ref']);
const defaultOverviewReadings = ['operating_data.external_temp','operating_data.air_flow'];
let chosenMetrics = saved('metrics',defaultOverviewReadings)
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
function titleSignal(key) {
  if (key.startsWith('home_assistant.')) return status.haSignals?.[key]?.name ?? textLabel(key.slice(17));
  return textLabel(readingName(key));
}
function unitSignal(key) {
  const externalUnit = status.haSignals?.[key]?.unit;
  if (externalUnit) return externalUnit;
  if (key.startsWith('home_assistant.t_')) return '°C';
  if (key.startsWith('home_assistant.f_')) return 'm³/h';
  return labels[readingName(key)]?.[1] ?? '';
}
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
  if (status.historyRevision!==previous.historyRevision||(page==='overview'&&status.fahrenheit!==previous.fahrenheit)) refreshPageHistory?.();
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
  const powerPercent = signal('operating_data.power_pct');
  const powerHeading = op.state===5 ? [
    Number.isFinite(powerPercent) ? display(powerPercent)+'%' : null,
    Number.isFinite(output) ? decimalTenths.format(output)+' kW' : null,
  ].filter(Boolean) : [];
  const water = signal('operating_data.dhw_temp');
  const returnTemp = signal('operating_data.return_temp');
  const used = signal('advanced_data.total_consumption_24hours');
  const readings = [
    ['Fuel hopper',remaining!=null&&remaining<0 ? 'Below minimum' : withUnit(remaining,'kg'),
      used==null ? '24 h usage unavailable' : withUnit(used,'kg')+' used in 24 h','#fuel'],
    ['Boiler',withUnit(output,'kW'),withUnit(powerPercent,'%')+' burner output','#boiler'],
    ['Heating circuit',withUnit(returnTemp,'°C'),'Return water','#weather'],
    ['DHW',water==null?'No reading':withUnit(water,'°C'),'Cylinder sensor','#hot-water'],
  ];
  live.replaceChildren(dom('section',{class:'overview-system','aria-label':'Current boiler state'},
    dom('div',{class:'system-main'},
      dom('div',{},dom('div',{class:'operating-mode'},dom('span',{class:'indicator'+(status.online?' live':'')}),status.online?'Controller online':'Controller offline'),
        status.online && dom('h2',{class:powerHeading.length?'system-firing-state':null},op.state_text || 'Waiting for data',
          powerHeading.map(value=>dom('span',{class:'system-firing-output'},' '+value))),
        status.online ? dom('p',{id:'overview-phase',class:'muted'}) :
          dom('p',{class:'muted mt-1'},last ? 'Last updated '+new Date(last).toLocaleString(undefined,{month:'short',day:'numeric',hour:'numeric',minute:'2-digit'}) : 'Waiting for controller data.')),
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
  updateOverviewPhase();
}

const boilerSubstates = [null, 'Ventilating', 'Feeding pellets', 'Ignition pulse', 'Ignition',
  'Cooling boiler', 'Cooling burner', 'Fan cleaning', 'Ash removal',
  'Compressor cleaning: waiting for valve 1', 'Compressor cleaning: valve 1 active',
  'Compressor cleaning: waiting for valve 2', 'Compressor cleaning: valve 2 active',
  'Compressor cleaning: waiting for valve 3', 'Compressor cleaning: valve 3 active',
  'External contact', 'Overrun', 'Residual heat to hot water', 'Vacuum transport',
  'Airflow calibration: waiting for shutdown', 'Calibrating airflow', 'Calculated efficiency'];

function updateOverviewPhase() {
  const line = document.getElementById('overview-phase');
  if (!line) return;
  const {at, values} = status.data.operating_data ?? {};
  const {substate,substate_sec:seconds} = values ?? {};
  let phase = '';
  if (Number.isInteger(substate)&&substate>0) {
    phase = boilerSubstates[substate] ?? 'Sub-state '+substate;
    if (Number.isFinite(seconds)&&seconds>0) {
      if (substate===21) {
        phase += ' · '+decimalTenths.format(seconds/10)+'%';
      } else {
        const sampledAt = Date.parse(at);
        const elapsed = Number.isFinite(sampledAt) ? Math.max(0,Math.floor((Date.now()-sampledAt)/1000)) : 0;
        const left = Math.ceil(seconds-elapsed);
        if (left>0) phase += ' · '+Math.floor(left/60)+':'+String(left%60).padStart(2,'0')+' remaining';
      }
    }
  }
  line.hidden = !phase;
  if (line.textContent!==phase) line.textContent=phase;
}
function withUnit(value,unit) {
  return display(value)+(value==null||!unit?'':' '+unit);
}
function renderCombustion(target) {
  const reading = (name,key,className='combustion-reading')=>{
    const value = status.online ? signal('operating_data.'+key) : null;
    const formatted = key==='oxygen'&&typeof value==='number' ? decimalTenths.format(value) : display(value);
    return dom('div',{class:className},
      dom('span',{class:'metric-label'},name),
      dom('strong',{class:'combustion-value'},formatted,value!=null&&dom('small',{},unitSignal(key))));
  };
  const oxygen = reading('Oxygen','oxygen','combustion-current');
  const oxygenTarget = status.online ? signal('operating_data.oxygen_ref') ?? signal('advanced_data.o2_setpoint') : null;
  if(snapshot('operating_data').state_on!=='OFF'&&oxygenTarget>0){
    const label=oxygen.firstElementChild;
    const header=dom('div',{class:'combustion-oxygen-label'});
    label.replaceWith(header);
    header.append(label,dom('span',{class:'combustion-target'},'Target '+decimalTenths.format(oxygenTarget)+' %'));
  }
  const compensation = [['Low','corr_low'],['Medium','corr_medium'],['High','corr_high']];
  target.replaceChildren(
    dom('section',{class:'combustion-reading combustion-oxygen','aria-label':'Oxygen and compensation'},
      oxygen,
      dom('div',{class:'combustion-compensation'},dom('span',{class:'metric-label'},'O₂ compensation'),
        dom('dl',{},compensation.map(([name,key])=>{
          const value = status.online ? signal('operating_data.'+key) ?? signal('advanced_data.o2_'+key) : null;
          return dom('div',{},dom('dt',{},name),dom('dd',{},withUnit(value,'%')));
        })))),
    reading('Photo sensor','photo_level'),
    reading('Chimney smoke','smoke_temp'));
}
function overview(token) {
  const live = dom('div',{id:'overview-live'});
  const chartBody = dom('div',{},empty('Loading performance history…'));
  const recent = dom('div',{},empty('Loading recent events…'));
  const metricsPanel = dom('div',{},empty('Waiting for telemetry…'));
  const customise = dom('details',{},dom('summary',{},'Choose overview readings'),metricsPanel,
    btn('Reset to default',()=>setMetrics(null),'secondary mt-2'));
  main.replaceChildren(heading('Pellet boiler'),
    dom('div',{class:'stack'},live,
      card('Combustion',null,dom('div',{class:'combustion-grid',id:'overview-combustion'})),
      dom('div',{class:'grid split'},
        card('Performance','Last 24 hours',chartBody,dom('a',{class:'home-link',href:'#charts'},'All charts')),
        card('Recent events',null,recent,dom('a',{class:'home-link',href:'#events'},'View all events'))),
      card('Other readings',null,dom('div',{class:'compact-readings',id:'overview-custom'}),customise)));
  updateOverview();
  overviewPhaseTimer = setInterval(updateOverviewPhase,1000);
  const setMetrics = selection=>{
    chosenMetrics=selection??defaultOverviewReadings;
    saveLocal('metrics',selection);
    updateOverview();
    renderChooser();
  };
  const renderChooser = ()=>chooseSignals(metricsPanel,chosenMetrics,setMetrics,{exclude:combustionReadings});
  customise.addEventListener('toggle',()=>{if(customise.open)renderChooser();});
  const performanceNames = {
    'operating_data.boiler_temp':'Boiler temperature',
    'operating_data.boiler_ref':'Temperature target',
    'operating_data.oxygen':'Measured O₂',
    'operating_data.oxygen_ref':'O₂ target',
  };
  const performanceColors = {
    'operating_data.boiler_temp':colors[0],
    'operating_data.boiler_ref':'#6b85c5',
    'operating_data.oxygen':colors[1],
    'operating_data.oxygen_ref':'#3f9b87',
  };
  let historyRequest = 0;
  const refreshPerformance = async(showError=false)=>{
    const request = ++historyRequest;
    try {
      const data=await api('history/performance');
      if(token!==generation||request!==historyRequest)return;
      const ordered=Object.fromEntries(Object.keys(performanceNames).map(key=>[key,data[key]??[]]));
      showChart(chartBody,ordered,86400,{size:'performance',names:performanceNames,
        seriesColors:performanceColors,axes:{'°C':null,'%':{minY:0,maxY:status.fahrenheit?50:100}},maxGap:60});
    }catch(error){
      if(showError&&token===generation&&request===historyRequest)chartBody.replaceChildren(empty(error.message));
    }
  };
  refreshPageHistory = refreshPerformance;
  refreshPerformance(true);
  let recentRequest = 0;
  let recentLoaded = false;
  const refreshRecent = async()=>{
    const request = ++recentRequest;
    try {
      const items = await api('events');
      if(token!==generation||request!==recentRequest)return;
      recent.replaceChildren(eventList(items.slice(0,5)));
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
  for (const key of Object.keys(status.haSignals ?? {})) keys.add(key);
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
  target.replaceChildren(keys.length ? chooser : empty('Measurements will appear after the first reading.'));
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
function charts(token, initialSignals) {
  let chartSignals = initialSignals?.length ? initialSignals : chosenSignals;
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
    if (!chartSignals.length) {
      chartBody.replaceChildren(empty('Choose up to eight signals to chart.'));
      csv.disabled=true; return;
    }
    if(showLoading) {
      csv.disabled=true;
      chartBody.replaceChildren(empty('Loading history…'));
    }
    try {
      const loaded = await loadHistory(chartSignals,duration);
      if(token!==generation||request!==id)return;
      data=loaded;
      showChart(chartBody,data,duration);
      csv.disabled=!Object.values(data).some(points=>points.length);
    } catch(error) {
      if(request===id&&token===generation&&showLoading)chartBody.replaceChildren(empty(error.message));
    }
  };
  const choose = ()=>chooseSignals(signals,chartSignals,selection=>{
    chartSignals=chosenSignals=selection; saveLocal('signals',selection); choose(); load();
  },{limit:8});
  main.replaceChildren(heading('Charts','Compare live signals recorded by Boiler Mate. History starts when this instance is enabled.'),
    controllerTabs('charts','recorded'),card('Signal history','Each signal keeps its own unit.',
      dom('div',{class:'toolbar'},dom('label',{},'Period',range),csv),
      chartBody,dom('details',{open:true},dom('summary',{},'Select signals'),signals)));
  let available = JSON.stringify(measurementKeys(chartSignals));
  refreshPageHistory = ()=>{
    const updated = JSON.stringify(measurementKeys(chartSignals));
    if (available!==updated) {
      available=updated;
      choose();
    }
    load(false);
  };
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
function showChart(target,data,duration,{size='',names={},seriesColors={},axes={},maxGap=Infinity,endTime,sampleOrder=false}={}) {
  const entries = Object.entries(data).filter(([,points])=>points.length);
  chartRenderers.delete(target);
  if(!entries.length){target.replaceChildren(empty('No readings in this period yet.'));return;}
  const previous=target.querySelector('canvas.chart');
  const focused=previous===document.activeElement;
  const previousTime=previous?.dataset.cursorTime;
  const label=key=>names[key]??titleSignal(key);
  const color=(key,i)=>seriesColors[key]??colors[i];
  const units = new Set(entries.map(([key])=>unitSignal(key)));
  const axisUnits=Object.keys(axes).filter(unit=>units.has(unit));
  const mixed=units.size>1&&!axisUnits.length;
  const performance=size==='performance';
  const visual = dom('canvas',{class:size?'chart '+size:'chart',role:'img',tabindex:0,
    'aria-label':(performance?'Performance, 24 hours. ':'Line chart. ')+entries.map(([key])=>label(key)).join(', ')+'. Use the arrow keys to inspect values.'});
  const cursor=dom('div',{class:'chart-cursor',hidden:true,'aria-hidden':'true'});
  const tooltip=dom('div',{class:'chart-tooltip',hidden:true,role:'status'});
  const plot=dom('div',{class:'chart-plot'},visual,cursor,tooltip);
  const samples=new Map();
  const swatch=(key,i)=>{
    const dot=dom('span',{class:key.endsWith('_ref')?'dot dashed':'dot','aria-hidden':'true'});
    dot.style.color=color(key,i);
    return dot;
  };
  entries.forEach(([key,points],i)=>{
    for(const {time,value} of points){
      if(!samples.has(time))samples.set(time,[]);
      samples.get(time).push({key,value,i});
    }
  });
  const times=[...samples.keys()].sort((a,b)=>a-b);
  const legend = dom('div',{class:'legend'},entries.map(([key,points],i)=>{
    const text=label(key)+(performance?'':' · '+withUnit(points.at(-1).value,unitSignal(key)));
    return dom('span',{},swatch(key,i),text);
  }));
  target.replaceChildren(plot,legend);
  if (mixed) target.append(dom('p',{class:'muted'},'Signals with the same unit share a scale.'));
  let view;
  let index=previousTime ? nearestChartIndex(times,Number(previousTime)) : null;
  const select=next=>{
    index=next;
    if(next==null){
      cursor.hidden=tooltip.hidden=true;
      delete visual.dataset.cursorTime;
      return;
    }
    if(!view)return;
    const time=times[next];
    if(visual.dataset.cursorTime!==String(time)){
      tooltip.replaceChildren(dom('div',{class:'chart-tooltip-time'},sampleOrder?'Sample '+time:datetime.format(new Date(time*1000))),
        ...samples.get(time).map(({key,value,i})=>dom('div',{class:'chart-tooltip-row'},
          dom('span',{},swatch(key,i),label(key)),dom('strong',{},withUnit(value,unitSignal(key))))));
      visual.dataset.cursorTime=time;
    }
    cursor.hidden=tooltip.hidden=false;
    const {x0,x1,bounds}=view;
    const x=x0+(time-bounds.minX)*(x1-x0)/(bounds.maxX-bounds.minX);
    cursor.style.left=x+'px';
    const tooltipWidth=tooltip.offsetWidth;
    const left=x+12+tooltipWidth<=plot.clientWidth ? x+12 : Math.max(0,x-12-tooltipWidth);
    tooltip.style.left=left+'px';
  };
  const pointAt=event=>{
    if(!view)return;
    const {x0,x1,bounds}=view;
    const x=event.clientX-visual.getBoundingClientRect().left;
    const fraction=Math.max(0,Math.min(1,(x-x0)/(x1-x0)));
    const nearest=nearestChartIndex(times,bounds.minX+fraction*(bounds.maxX-bounds.minX));
    if(nearest!==index)select(nearest);
  };
  visual.addEventListener('pointermove',pointAt);
  visual.addEventListener('pointerdown',pointAt);
  visual.addEventListener('pointerleave',event=>{if(event.pointerType!=='touch')select(null);});
  visual.addEventListener('blur',()=>select(null));
  visual.addEventListener('keydown',event=>{
    switch(event.key){
      case 'ArrowLeft':select(index==null?times.length-1:Math.max(0,index-1));break;
      case 'ArrowRight':select(index==null?0:Math.min(times.length-1,index+1));break;
      case 'Home':select(0);break;
      case 'End':select(times.length-1);break;
      case 'Escape':select(null);break;
      default:return;
    }
    event.preventDefault();
  });
  const render = ()=>{
    if(!visual.isConnected)return;
    const width=visual.clientWidth,height=visual.clientHeight,dpr=window.devicePixelRatio||1;
    if(!width||!height)return;
    visual.width=Math.round(width*dpr);visual.height=Math.round(height*dpr);
    const ctx=visual.getContext('2d');
    ctx.scale(dpr,dpr);
    const bounds=chartBounds(entries.map(([,points])=>points));
    if(!sampleOrder){
      const newest=endTime??Math.floor(Date.now()/1000);
      bounds.minX=Math.min(bounds.minX,newest-duration);
      bounds.maxX=Math.max(bounds.maxX,newest);
    }
    const step=chartStep((bounds.maxY-bounds.minY)/4);
    bounds.minY=Math.floor(bounds.minY/step)*step;
    bounds.maxY=Math.ceil(bounds.maxY/step)*step;
    const rows=axisUnits.length?5:mixed?4:Math.min(8,Math.round((bounds.maxY-bounds.minY)/step));
    const scales=new Map(axisUnits.map(unit=>{
      if(axes[unit])return [unit,axes[unit]];
      const range=chartBounds(entries.filter(([key])=>unitSignal(key)===unit).map(([,points])=>points));
      const axisStep=chartStep((range.maxY-range.minY)/(rows-1));
      const minY=Math.floor(range.minY/axisStep)*axisStep;
      return [unit,{minY,maxY:minY+rows*axisStep}];
    }));
    if (mixed) {
      for (const unit of units) {
        scales.set(unit,chartBounds(entries.filter(([key])=>unitSignal(key)===unit).map(([,points])=>points)));
      }
    }
    const x0=axisUnits.length?48:42, x1=width-(axisUnits.length>1?44:16), y0=14, y1=height-29;
    view={x0,x1,bounds};
    ctx.font='11px system-ui';ctx.fillStyle='#556773';
    for(let i=0;i<=rows;i++){
      const y=y1-i*(y1-y0)/rows;
      ctx.strokeStyle='#dce3dd';ctx.lineWidth=1;ctx.beginPath();ctx.moveTo(x0,y);ctx.lineTo(x1,y);ctx.stroke();
      if(axisUnits.length){
        for(const [index,unit] of axisUnits.entries()){
          const {minY,maxY}=scales.get(unit);
          const tick=numeric.format(minY+(maxY-minY)*i/rows);
          ctx.textAlign=index?'right':'left';
          ctx.fillText(tick+(unit==='%'?'%':' '+unit),index?width-2:2,y+3);
        }
      }else if(!mixed)ctx.fillText(numeric.format(bounds.minY+i*step),2,y+3);
    }
    const formatTime = duration<=86400?shorttime:new Intl.DateTimeFormat(undefined,
      duration>=365*86400?{month:'short',year:'numeric'}:{month:'short',day:'numeric'});
    const ticks=sampleOrder?times.filter((_,i)=>i===0||i===times.length-1||
      i%Math.max(1,Math.ceil(times.length*90/(x1-x0)))===0):chartTimeTicks(bounds.minX,bounds.maxX,x1-x0);
    for(const [i,tick] of ticks.entries()){
      const x=x0+(tick-bounds.minX)*(x1-x0)/(bounds.maxX-bounds.minX);
      if(i>0&&i<ticks.length-1){
        ctx.strokeStyle='#edf0f4';ctx.beginPath();ctx.moveTo(x,y0);ctx.lineTo(x,y1);ctx.stroke();
      }
      ctx.textAlign=i===0?'left':i===ticks.length-1?'right':'center';
      ctx.fillText(sampleOrder?'#'+tick:formatTime.format(new Date(tick*1000)),x,height-6);
    }
    ctx.save();
    ctx.beginPath();
    ctx.rect(x0,y0,x1-x0,y1-y0);
    ctx.clip();
    for(const [i,[key,points]] of entries.entries()){
      const own=scales.get(unitSignal(key))??bounds;
      ctx.strokeStyle=color(key,i);ctx.fillStyle=color(key,i);ctx.lineWidth=2.2;
      ctx.setLineDash(key.endsWith('_ref')?[5,4]:[]);ctx.beginPath();
      const isolated=[];
      for(const [j,p] of points.entries()){
        const x=x0+(p.time-bounds.minX)*(x1-x0)/(bounds.maxX-bounds.minX);
        const y=y1-(p.value-own.minY)*(y1-y0)/(own.maxY-own.minY);
        const follows=j>0&&p.time-points[j-1].time<=maxGap;
        if(follows)ctx.lineTo(x,y);else ctx.moveTo(x,y);
        if(!follows&&(j===points.length-1||points[j+1].time-p.time>maxGap))isolated.push([x,y]);
      }
      ctx.stroke();
      ctx.beginPath();
      for(const [x,y] of isolated){
        ctx.moveTo(x+2.5,y);
        ctx.arc(x,y,2.5,0,Math.PI*2);
      }
      ctx.fill();
    }
    ctx.restore();
    select(index);
  };
  requestAnimationFrame(render);
  chartRenderers.set(target,render);
  if(focused)visual.focus({preventScroll:true});
}
function nearestChartIndex(times,value) {
  let left=0,right=times.length-1;
  while(left<right){
    const middle=Math.floor((left+right)/2);
    if(times[middle]<value)left=middle+1;
    else right=middle;
  }
  return left>0&&value-times[left-1]<=times[left]-value?left-1:left;
}
function chartTimeTicks(start,end,width) {
  const spacing=70;
  const range=end-start;
  const ideal=range/Math.max(1,Math.floor(width/spacing));
  const steps=[60,300,600,900,1800,3600,7200,10800,14400,21600,28800,43200,
    86400,2*86400,4*86400,7*86400,14*86400,30*86400,60*86400,120*86400,365*86400];
  const step=steps.find(value=>value>=ideal)??Math.ceil(ideal/(365*86400))*365*86400;
  const edge=range*spacing*.65/Math.max(1,width);
  const ticks=[start];
  for(let tick=Math.ceil((start+edge)/step)*step;tick<=end-edge;tick+=step)ticks.push(tick);
  ticks.push(end);
  return ticks;
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
  const selector=renderSections('consumption',consumptionSections,period,'Consumption sections');
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
    pill(textLabel(row.kind),row.kind==='alarm'?'error':['connection','fuel'].includes(row.kind)?'warn':''),
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
      ['connection','Connection'],['fuel','Pellet supply'],['state','State changes'],['command','Commands']]
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
  main.replaceChildren(heading('Events'),dom('div',{class:'stack'},controllerTabs('events','activity'),
    dom('section',{class:'card'},desktopNotifications(notificationsOpen)),
    dom('section',{class:'card'},dom('div',{class:'toolbar'},filter),entries,more)));
  refreshPageEvents=refreshEvents;
  load('reset');
}
function desktopNotifications(open) {
  const supportsPush=webPushAvailable();
  const description=dom('p');
  let setup=null;
  const desktop=btn('',async()=>{
    desktop.disabled=true;
    inputs.disabled=true;
    try {
      if(supportsPush){
        if(pushEnabled()&&await activePushSubscription())await disableWebPush();
        else await enableWebPush(setup);
      }else if(!desktopNoticesAllowed()){
        if(typeof Notification==='undefined'||!window.isSecureContext)throw new Error('Desktop notices need a supported browser and HTTPS.');
        const permission=await Notification.requestPermission();
        if(permission!=='granted')throw new Error('Your browser did not grant notification access.');
      }
    }catch(error){
      toast(error.message);
    }finally{
      inputs.disabled=false;
      updateControls();
      connectDesktopNotices();
    }
  },'primary');
  const updateControls=()=>{
    if(supportsPush){
      const active=pushEnabled();
      description.textContent=active?'Notices can arrive even after you close the dashboard tab.':'Enable desktop notices to receive them even with the dashboard tab closed.';
      desktop.textContent=active?'Disable closed-tab notices':desktopNoticesAllowed()?'Enable closed-tab notices':'Enable desktop notices';
      desktop.disabled=!setup;
    }else{
      description.textContent='Choose which notices to show while the dashboard tab is open, including in the background.';
      desktop.textContent=desktopNoticesAllowed()?'Desktop notifications enabled':'Enable desktop notices';
      desktop.disabled=desktopNoticesAllowed();
    }
  };
  const options=[['alarm','Boiler alarms'],['info','Controller information'],['connection','Connection interruptions'],['fuel','Pellet supply']];
  const inputs=dom('fieldset',{class:'signal-chooser','aria-label':'Desktop notice categories'},options.map(([kind,label])=>dom('label',{},
    dom('input',{type:'checkbox',checked:noticeFilter.includes(kind),onchange:async event=>{
      const checkbox=event.target;
      const kinds=checkbox.checked?[...noticeFilter,kind]:noticeFilter.filter(item=>item!==kind);
      inputs.disabled=true;
      try {
        if(pushEnabled())await saveWebPush(await activePushSubscription(),kinds);
        noticeFilter=kinds;
        saveLocal('noticeFilter',kinds);
      }catch(error){
        checkbox.checked=noticeFilter.includes(kind);
        toast(error.message);
      }finally{
        inputs.disabled=false;
      }
    }}),label)));
  const panel=dom('details',{open},dom('summary',{},'Desktop notifications'),description,inputs,desktop);
  const prepare=async()=>{
    if(!supportsPush||setup)return;
    try {
      const config=await prepareWebPush();
      if(pushEnabled())await activePushSubscription();
      setup=config;
    }catch{
      description.textContent='Could not load desktop notification settings. Reopen this panel to retry.';
      return;
    }
    updateControls();
  };
  panel.addEventListener('toggle',()=>{if(panel.open)prepare();});
  updateControls();
  if(open)prepare();
  return panel;
}
function desktopNoticesAllowed() {
  return typeof Notification!=='undefined'&&Notification.permission==='granted';
}
function webPushAvailable() {
  return window.isSecureContext && 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
}
function pushEnabled() {
  return webPushAvailable() && saved('closedTabNotices',Boolean(pushSubscription));
}
function prepareWebPush() {
  if(!pushSetup){
    const key=api('notifications/push-key');
    const worker=navigator.serviceWorker.register('/notice-worker.js',{updateViaCache:'none'});
    pushSetup=Promise.all([key,worker]).then(async([settings])=>({
      key:Uint8Array.from(atob(settings.publicKey.replace(/-/g,'+').replace(/_/g,'/')),char=>char.charCodeAt(0)),
      registration:await navigator.serviceWorker.ready,
    })).catch(error=>{
      pushSetup=null;
      throw error;
    });
  }
  return pushSetup;
}
async function activePushSubscription() {
  const {registration}=await prepareWebPush();
  const subscription=await registration.pushManager.getSubscription();
  pushSubscription=subscription;
  if(!subscription)saveLocal('closedTabNotices',false);
  return subscription;
}
async function enableWebPush({registration,key}) {
  // Start the subscription from the button click to preserve the browser’s user activation.
  const subscription=await registration.pushManager.subscribe({userVisibleOnly:true,applicationServerKey:key});
  try {
    await saveWebPush(subscription,noticeFilter);
  }catch(error){
    await subscription.unsubscribe().catch(()=>{});
    throw error;
  }
  pushSubscription=subscription;
  saveLocal('closedTabNotices',true);
  toast('Desktop notices are enabled, even with the tab closed.');
}
async function disableWebPush() {
  const {endpoint}=pushSubscription;
  await pushSubscription.unsubscribe();
  pushSubscription=null;
  saveLocal('closedTabNotices',false);
  await api('notifications/push',{method:'DELETE',body:JSON.stringify({endpoint})});
  toast('Closed-tab notices are disabled.');
}
async function saveWebPush(subscription,kinds) {
  if(!subscription)throw new Error('Enable closed-tab notices again before changing their categories.');
  const {endpoint,keys}=subscription.toJSON();
  await api('notifications/push',{method:'PUT',body:JSON.stringify({endpoint,keys,kinds})});
}
function deliverDesktopNotices(rows,cursor) {
  if(!desktopNoticesAllowed()||pushEnabled()){
    stopDesktopNotices();
    lastDesktopNotice=null;
    return;
  }
  const previous=lastDesktopNotice;
  lastDesktopNotice=cursor;
  if(previous===null)return;
  const selected=saved('noticeFilter',noticeFilter);
  for(const row of rows){
    if(row.id<=previous||row.read||!selected.includes(row.kind))continue;
    try {
      new Notification('Boiler Mate',{body:row.message,icon:'/icon-192.png',tag:'boiler-'+row.id});
    } catch {}
  }
}
function startNoticePolling() {
  if(noticePoll)return;
  let pending=false;
  const check=async()=>{
    if(pending)return;
    pending=true;
    try {
      const rows=await api('events?filter=notices');
      const cursor=Math.max(lastDesktopNotice??0,rows[0]?.id??0);
      if(noticePoll===poll)deliverDesktopNotices(rows.reverse(),cursor);
    } catch {}
    finally {pending=false;}
  };
  const poll=setInterval(check,15000);
  noticePoll=poll;
  check();
}
function stopDesktopNotices() {
  noticeSource?.close();
  noticeSource=null;
  clearInterval(noticePoll);
  noticePoll=0;
}
function connectDesktopNotices() {
  if(!desktopNoticesAllowed()||pushEnabled()){
    stopDesktopNotices();
    lastDesktopNotice=null;
    return;
  }
  if(typeof EventSource!=='function'){
    startNoticePolling();
    return;
  }
  if(noticeSource?.readyState===EventSource.CLOSED)stopDesktopNotices();
  if(noticeSource)return;
  const after=lastDesktopNotice===null?'':'?after='+lastDesktopNotice;
  const source=new EventSource('/api/notifications/stream'+after);
  noticeSource=source;
  source.onmessage=event=>{
    if(noticeSource!==source)return;
    const cursor=Number(event.lastEventId);
    let rows;
    try {rows=JSON.parse(event.data);}
    catch {return;}
    if(Number.isSafeInteger(cursor)&&Array.isArray(rows))deliverDesktopNotices(rows,cursor);
  };
  source.onerror=()=>{
    if(noticeSource===source&&source.readyState===EventSource.CLOSED)startNoticePolling();
  };
}
function dataTable(query) {
  const entries=liveReadingKeys().sort((a,b)=>titleSignal(a).localeCompare(titleSignal(b)));
  if(!entries.length)return empty('No live readings available.');
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
  main.replaceChildren(heading('Live data'),controllerTabs('live-data','all'),dom('section',{class:'card'},
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
    id==='fuel'&&dom('a',{class:'secondary',href:'#consumption/ledger'},'Pellet purchase ledger'),
    btn('Refresh readings',async()=>{await refresh();renderPage();}));
  main.replaceChildren(heading(title,description,actions),
    dom('div',{class:'stack'},
      id==='system'&&controllerTabs('system','controller'),
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

function efficiencyPage(token) {
  const content=dom('div',{class:'stack','aria-live':'polite'},empty('Loading observations…'));
  main.replaceChildren(heading('Efficiency suggestions',
    'Review repeated patterns in burner runs, thermostat demand, and temperature regulation.'),
    renderSections('efficiency',efficiencySections,'suggestions','Efficiency views'),content);
  let lastRequest=0;
  async function load() {
    const request=++lastRequest;
    try {
      const report=await api('efficiency');
      if (token!==generation || request!==lastRequest) return;
      const last=report.lastSample && new Date(report.lastSample);
      const fresh=last && Date.now()-last.valueOf()<5*60000;
      const observed=report.heatDemandMinutes;
      const duration=observed<60 ? observed+' min' : decimalTenths.format(observed/60)+' hr';
      const stats=dom('section',{class:'efficiency-overview','aria-label':'Observation coverage'},
        dom('div',{class:'efficiency-status'},dom('h2',{},'Last 7 days'),
          pill(fresh?'Observing':last?'Awaiting fresh telemetry':'Waiting for observations',fresh?'good':'')),
        dom('dl',{class:'efficiency-metrics'},[
          ['Continuous heat demand observed',duration],
          ['Firing runs observed',display(report.observedRuns)],
          ['Latest observation',last?datetime.format(last):'—'],
        ].map(([label,value])=>dom('div',{},dom('dt',{},label),dom('dd',{},value)))));
      const suggestions=report.suggestions ?? [];
      const results=suggestions.length ? suggestions.map(suggestion=>
        dom('article',{class:'efficiency-suggestion'},
          dom('div',{class:'efficiency-suggestion-head'},
            dom('h2',{},suggestion.title),
            dom('time',{datetime:suggestion.latest},datetime.format(new Date(suggestion.latest)))),
          dom('p',{class:'efficiency-zones'},suggestion.zones),
          dom('p',{},suggestion.summary),
          dom('p',{class:'efficiency-review'},dom('strong',{},'What to check'),suggestion.review))) :
        [card('Watching for repeat patterns',null,empty(observed>0 ?
          'Boiler Mate needs repeat observations under similar conditions before suggesting a change.' :
          'Boiler Mate needs a firing run and known heat demand from your selected thermostats.'),
        observed===0 && dom('a',{class:'home-link',href:'#external-heating'},'View thermostat signals'))];
      content.replaceChildren(stats, ...results,
        dom('p',{class:'muted'},'Suggestions are advisory. Boiler Mate uses comparable space-heating observations and leaves controller settings to you.'));
    } catch (error) {
      if (token===generation && request===lastRequest) content.replaceChildren(empty(error.message));
    }
  }
  refreshPageHistory=load;
  load();
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
  clearInterval(overviewPhaseTimer);
  chartRenderers.clear();
  const [hashPath,parameters]=window.location.hash.slice(1).split('?');
  const [hashPage,requestedSection]=hashPath.split('/');
  const requested=({ 'advanced-data':'live-data', messages:'events', notifications:'events' })[hashPage]??hashPage;
  page=navigation.flatMap(([,items])=>items).some(([id])=>id===requested)?requested:'overview';
  const sections=groups[page]?.[2] ?? (page==='consumption'?consumptionSections:[]);
  const [section,sectionLabel]=sections.find(([id,,slug=id])=>slug===requestedSection) ?? sections[0] ?? [];
  const token=++generation;
  const readSection=controllerSections[page]?.slice(1).find(([kind])=>kind===requestedSection);
  renderNavigation(readSection?.[1]??(requestedSection?sectionLabel:null));
  if(readSection)controllerReadPage(page,readSection[0],token);
  else if (groups[page]) settingsPage(page,section,token);
  else if (page==='live-data') readingsPage();
  else if (page==='overview') overview(token);
  else if (page==='charts') {
    const signals = new URLSearchParams(parameters).getAll('series').filter(key=>/^[a-z0-9_.]{1,100}$/.test(key));
    charts(token,[...new Set(signals)].slice(0,8));
  }
  else if (page==='consumption') {
    if(section==='ledger')pelletLedgerPage(token);
    else consumption(section,token);
  }
  else if (page==='efficiency' && requestedSection==='weather') degreeWeatherPage(token);
  else if (page==='efficiency') efficiencyPage(token);
  else if (page==='events') activity(token,hashPage==='messages'?'notices':'',hashPage==='notifications'||requestedSection==='notifications');
  else if (page==='external-heating') externalHeatingPage(token);
  else helpPage();
}

document.querySelector('.skip').addEventListener('click',event=>{event.preventDefault();main.focus();});
window.addEventListener('hashchange',()=>{if(window.location.hash!=='#content')renderPage();});
window.addEventListener('resize',()=>chartRenderers.forEach(render=>render()));
function connectPageStreams() {
  connectWhileVisible();
  connectDesktopNotices();
}
if(pushEnabled())activePushSubscription().then(connectDesktopNotices).catch(()=>{});
window.addEventListener('storage',event=>{
  if(event.key==='boilermate.closedTabNotices')connectDesktopNotices();
  if(event.key==='boilermate.noticeFilter')noticeFilter=saved('noticeFilter',noticeFilter);
});
document.addEventListener('visibilitychange',connectPageStreams);
document.addEventListener('freeze',stopDesktopNotices);
document.addEventListener('resume',connectDesktopNotices);
window.addEventListener('pagehide',()=>{
  stopStatusStream();
  stopDesktopNotices();
});
window.addEventListener('pageshow',connectPageStreams);
refresh().then(()=>{renderPage();connectPageStreams();});
