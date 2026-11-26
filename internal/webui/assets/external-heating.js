'use strict';

async function externalHeatingPage(pageID, token) {
  const zones = pageID==='zones';
  const label = zones ? 'Zones' : 'Outdoor Climate';
  const title = heading(label, zones ? 'Choose thermostats for heat demand and link zone pump readings.' :
    'Set when outdoor temperatures stop the boiler or limit burner power.');
  main.replaceChildren(title, card(label+' Settings', null, empty('Loading settings…')));
  try {
    const data = await api('external-heating');
    if (token===generation) externalHeatingForm(title, label, zones, data, token);
  } catch (error) {
    if (token===generation) {
      main.replaceChildren(title, card(label+' Settings', null, empty(error.message),
        btn('Try again', () => externalHeatingPage(pageID,token))));
    }
  }
}

function heatingField(label, control) {
  return dom('div',{class:'heating-field'},dom('label',{for:control.id},label),control);
}

function heatingNumber(id, value, attrs={}) {
  return dom('input',{id,type:'number',class:'field-input',value,...attrs});
}

function heatingPanel(label, description, action, ...children) {
  return dom('section',{class:'heating-panel'},
    dom('header',{class:'heating-panel-head'},dom('div',{},dom('h2',{},label),dom('p',{},description)),action),children);
}

function setHeatingText(element, text) {
  if (element.textContent!==text) element.textContent=text;
}

function heatingTemperature(value, from, to=from) {
  if (value==null) return '—';
  if (from!==to) value=to==='F' ? value*9/5+32 : (value-32)*5/9;
  return decimalTenths.format(value)+' °'+to;
}

function externalHeatingForm(title, label, zones, data, token) {
  let savedEnabled = data.settings.enabled;
  let saving = false;
  let revision = 0;
  const enabled = dom('input',{id:'heating-enabled',type:'checkbox',role:'switch',class:'heating-switch',
    checked:savedEnabled,'aria-describedby':'heating-control-scope'});
  const liveState = dom('span',{class:'heating-state'});
  const report = dom('p',{role:'status',class:'heating-decision'});
  const readOnly = dom('p',{class:'heating-readonly'},'Boiler writes and a controller PIN are required to enable control.');
  const resume = btn('Resume automatic control',async()=>{
    if (!await confirmChange('Resume automatic control?',
      'The boiler can start again according to the configured heating rules.', 'Resume control')) return;
    if (token!==generation) return;
    resume.disabled=true;
    revision++;
    try {
      const next=await api('external-heating/resume',{method:'POST'});
      if (token!==generation) return;
      revision++;
      show(next);
      toast('Automatic heating control resumed.');
    } catch (error) {
      toast(error.message);
    } finally {
      resume.disabled=false;
    }
  },'heating-resume',{hidden:true});

  const outdoorReading = dom('dd');
  const demandReading = dom('dd');
  const powerReading = dom('dd');
  const metrics = [
    ['Heat demand', demandReading], ['Outdoor temperature', outdoorReading], ['Target maximum power', powerReading],
  ].map(([name,value])=>dom('div',{},dom('dt',{},name),value));
  const notices = dom('div',{class:'heating-notices',role:'status'});
  const feedback = dom('p',{role:'alert',class:'heating-error'});
  const save = dom('button',{type:'submit',class:'primary'},'Save changes');
  const discard = btn('Discard',()=>externalHeatingForm(title,label,zones,data,token),'heating-quiet');
  const draftState = dom('span',{class:'heating-draft'});
  const saveDock = dom('div',{class:'heating-save'},
    dom('div',{},draftState,feedback),dom('div',{class:'heating-save-actions'},discard,save));
  const section = zones ? heatingZones(data,updateDraft) : outdoorClimate(data,updateDraft);
  const baseline = section.draft();

  function changes() {
    const draft = section.draft();
    const patch = {};
    for (const [key,value] of Object.entries(draft)) {
      if (JSON.stringify(value)!==JSON.stringify(baseline[key])) patch[key]=value;
    }
    // Save temperatures with their unit so a concurrent unit change cannot reinterpret them.
    const temperatures = ['unit','stopAt','restartDelta','bands'];
    if (temperatures.some(key=>Object.hasOwn(patch,key))) {
      for (const key of temperatures) patch[key]=draft[key];
    }
    if (enabled.checked!==savedEnabled) patch.enabled=enabled.checked;
    return patch;
  }

  function updateDraft() {
    const dirty = Object.keys(changes()).length>0;
    save.disabled = saving||!dirty;
    discard.disabled = saving;
    discard.hidden = !dirty;
    saveDock.dataset.dirty = String(dirty);
    setHeatingText(draftState,dirty ? 'Unsaved changes' : 'Settings saved');
  }

  function show(next) {
    data=next;
    if (enabled.checked===savedEnabled) enabled.checked=next.settings.enabled;
    savedEnabled=next.settings.enabled;
    enabled.disabled=!next.writable&&!savedEnabled;
    readOnly.hidden=next.writable;
    const automatic = savedEnabled&&next.writable;
    const held = automatic&&next.manualOff;
    liveState.dataset.state = held ? 'held' : automatic&&next.cutoffActive ? 'cutoff' : automatic ? 'active' : 'off';
    setHeatingText(liveState,held ? 'Manual stop' : automatic&&next.cutoffActive ? 'Outdoor cutoff active' : automatic ? 'Control active' : 'Control off');
    resume.hidden = !held;
    setHeatingText(report,savedEnabled ? next.decision : '');
    const selected = next.settings.thermostats ?? [];
    const reporting = (next.inputs ?? []).filter(input=>input.action&&selected.includes(input.id));
    const calling = reporting.filter(input=>input.action==='heating').length;
    setHeatingText(demandReading,selected.length ?
      (reporting.length ? calling+' of '+selected.length+' calling' : 'Waiting for state') : 'No thermostats selected');
    setHeatingText(outdoorReading,next.settings.outdoorSensor ?
      (next.outdoorTemperature==null ? 'Waiting for reading' : heatingTemperature(next.outdoorTemperature,next.settings.unit)) : 'No sensor selected');
    setHeatingText(powerReading,next.bandPower==null ? '—' : next.bandPower+'%');
    setHeatingText(notices,[next.error,next.powerError].filter(Boolean).join(' '));
    section.show(next);
    updateDraft();
  }

  async function submit(event) {
    event.preventDefault();
    const proposed=changes();
    if (saving||!Object.keys(proposed).length) return;
    saving=true;
    updateDraft();
    setHeatingText(feedback,'');
    try {
      if ((proposed.enabled ?? savedEnabled) && !await confirmChange('Apply external heating settings?',
        'Boiler Mate can start or stop the boiler and change its maximum burner power using these settings.', 'Save changes')) return;
      if (token!==generation) return;
      const next=await api('external-heating',{method:'PATCH',body:JSON.stringify(proposed)});
      if (token!==generation) return;
      toast(label+' settings saved.');
      externalHeatingForm(title,label,zones,next,token);
    } catch (error) {
      setHeatingText(feedback,error.message);
    } finally {
      saving=false;
      updateDraft();
    }
  }

  const form = dom('form',{class:'heating-page',onsubmit:submit},
    dom('section',{class:'heating-summary','aria-label':'External heating status'},
      dom('div',{class:'heating-summary-top'},
        dom('div',{},liveState,report,
          dom('p',{id:'heating-control-scope',class:'heating-decision'},'Automatic control applies to both Zones and Outdoor Climate.')),
        dom('div',{class:'heating-summary-actions'},resume,
          dom('label',{for:enabled.id,class:'heating-enable'},'Enable external control',enabled))),
      readOnly,dom('dl',{class:'heating-live'},metrics)),
    notices,section.element,saveDock);
  form.addEventListener('input',updateDraft);
  main.replaceChildren(title,form);
  show(data);

  async function refreshHeating() {
    if (!form.isConnected) return;
    const requestedRevision=revision;
    try {
      const next=await api('external-heating');
      if (form.isConnected&&revision===requestedRevision) show(next);
    } catch (error) {
      if (form.isConnected&&revision===requestedRevision) setHeatingText(report,error.message);
    }
    if (form.isConnected) setTimeout(refreshHeating,3000);
  }
  setTimeout(refreshHeating,3000);
}

function heatingZones(data, updateDraft) {
  const settings = data.settings;
  const selected = new Set(settings.thermostats ?? []);
  let inputsKey;
  const prefix = dom('input',{id:'heating-prefix',class:'field-input',type:'text',value:settings.discoveryPrefix,maxlength:128,required:true});
  const grace = heatingNumber('heating-grace',settings.dropoutSeconds,{min:5,max:86400,required:true});
  const cycle = heatingNumber('heating-cycle',settings.minCycleSeconds,{min:30,max:86400,required:true});
  const list = dom('div',{class:'heating-device-list'});
  const count = dom('span',{class:'heating-selected'});
  const pumps = zonePumpsPanel();

  function show(next) {
    data=next;
    const inputs = (next.inputs ?? []).filter(input=>input.kind==='thermostat');
    const discovered = new Map(inputs.map(input=>[input.id,input]));
    pumps.show(inputs,next.settings.thermostats ?? []);
    setHeatingText(count,selected.size+' selected');

    // Refresh readings in place to keep keyboard focus on the selected row.
    const key = JSON.stringify(inputs.map(({action,availability,temperature,...device})=>device));
    if (key!==inputsKey) {
      inputsKey = key;
      const missing = Array.from(selected).filter(id=>!discovered.has(id));
      list.replaceChildren(...[...inputs,...missing.map(id=>({id,name:id,problem:'Waiting for discovery'}))].map(input=>{
        const source = input.id.startsWith('ha:') ? 'Home Assistant' : 'MQTT';
        const box = dom('input',{type:'checkbox',class:'heating-device-check',checked:selected.has(input.id),
          disabled:Boolean(input.problem&&!selected.has(input.id)),'aria-label':input.name,
          onchange:()=>{
            if (box.checked) selected.add(input.id); else selected.delete(input.id);
            if (input.problem&&!box.checked) box.disabled=true;
            setHeatingText(count,selected.size+' selected');
            updateDraft();
          }});
        return dom('label',{class:'heating-device','data-device':input.id,title:source+' · '+input.id},box,
          dom('span',{class:'heating-device-label'},dom('strong',{},input.name),
            dom('span',{class:'heating-device-state','data-reading':'action'},input.problem)),
          dom('span',{class:'heating-device-temp','data-reading':'temperature','aria-label':'Room temperature'}));
      }));
      if (!list.children.length) list.append(empty('No thermostats available yet.'));
    }
    list.querySelectorAll('[data-device]').forEach(row=>{
      const input = discovered.get(row.dataset.device);
      const state = row.querySelector('[data-reading=action]');
      const value = input?.action;
      state.dataset.state = value==='heating' ? 'heating' : input?.availability==='offline' ? 'offline' : '';
      setHeatingText(state,input?.problem ?? (!input ? 'Waiting for discovery' :
        value ? value.charAt(0).toUpperCase()+value.slice(1) : input.availability==='offline' ? 'Offline' : 'Waiting for state'));
      setHeatingText(row.querySelector('[data-reading=temperature]'),heatingTemperature(input?.temperature,next.settings.unit));
    });
  }

  const clear = btn('Clear thermostat selections',()=>{
    selected.clear();
    inputsKey=undefined;
    show(data);
    updateDraft();
  },'heating-quiet');
  const advanced = dom('details',{class:'heating-advanced'},
    dom('summary',{},'Advanced Settings'),dom('div',{class:'heating-advanced-body'},
      dom('div',{class:'heating-field-pair'},heatingField('Dropout grace (seconds)',grace),heatingField('Minimum cycle (seconds)',cycle)),
      heatingField('MQTT discovery prefix',prefix),
      dom('p',{class:'heating-hint'},'Clear selections that use the old prefix here and in Outdoor Climate before changing it.'),
      clear));
  advanced.addEventListener('invalid',()=>{advanced.open=true;},true);
  const element = dom('div',{class:'heating-layout'},
    heatingPanel('Thermostats','Select every room that should be able to call for heat.',count,list),
    dom('div',{class:'heating-column'},pumps.element,advanced));
  return {element,show,draft:()=>({
    discoveryPrefix:prefix.value,thermostats:Array.from(selected),
    dropoutSeconds:grace.valueAsNumber,minCycleSeconds:cycle.valueAsNumber,
  })};
}

function outdoorClimate(data, updateDraft) {
  const settings = data.settings;
  let currentSensor = settings.outdoorSensor;
  let sourceUnit = settings.unit;
  let inputsKey;
  const timeout = heatingNumber('outdoor-timeout',settings.sensorTimeoutSeconds,{min:10,max:86400,required:true});
  const unit = dom('select',{id:'heating-unit',class:'field-input'},
    dom('option',{value:'C',selected:sourceUnit==='C'},'°C'),
    dom('option',{value:'F',selected:sourceUnit==='F'},'°F'));
  const stop = heatingNumber('outdoor-stop',settings.stopAt,{step:'any',placeholder:'Off','aria-describedby':'cutoff-hint'});
  const delta = heatingNumber('outdoor-delta',settings.restartDelta,{min:0.01,max:50,step:'any',required:true});
  const sensor = dom('select',{id:'heating-sensor',class:'field-input'});
  const cutoffHint = dom('p',{id:'cutoff-hint',class:'heating-hint'});
  const bands = dom('div',{class:'heating-bands'});
  const emptyBands = dom('p',{class:'heating-band-empty'},'Using the controller’s maximum burner power.');

  function temperature(value, from=data.settings.unit) {
    return heatingTemperature(value,from,unit.value);
  }

  function showSensors(next) {
    data=next;
    const inputs = (next.inputs ?? []).filter(input=>input.kind==='temperature');
    const discovered = new Map(inputs.map(input=>[input.id,input]));
    const key = JSON.stringify(inputs.map(({availability,temperature,...device})=>device));
    if (key!==inputsKey) {
      inputsKey=key;
      sensor.replaceChildren(dom('option',{value:''},'No outdoor sensor'),
        ...inputs.map(input=>dom('option',{value:input.id,disabled:Boolean(input.problem&&input.id!==currentSensor)},input.name)),
        currentSensor&&!discovered.has(currentSensor)&&dom('option',{value:currentSensor},currentSensor+' · Waiting for discovery'));
      sensor.value=currentSensor;
    }
    for (const option of sensor.options) {
      const input = discovered.get(option.value);
      if (input) {
        option.disabled=Boolean(input.problem&&input.id!==currentSensor);
        setHeatingText(option,input.name+(input.temperature!=null?' · '+temperature(input.temperature):'')+(input.problem?' · '+input.problem:''));
      }
    }
  }

  function syncBands() {
    const rows = Array.from(bands.children);
    for (const [index,row] of rows.entries()) {
      const {from,range,bounds} = row.controls;
      const next = rows[index+1]?.controls.from;
      from.hidden = index===0;
      from.required = index!==0;
      setHeatingText(range, index!==0 ? 'From (°'+unit.value+')' : rows.length===1 ? 'All temperatures' : 'Coldest band');
      setHeatingText(bounds, next&&next.value!=='' ? 'Below '+temperature(next.valueAsNumber,unit.value) :
        index!==0&&index===rows.length-1 ? 'And above' : '');
      row.classList.toggle('heating-band-base', index===0);
    }
    emptyBands.hidden = rows.length!==0;
    add.disabled = rows.length>=16;
    setHeatingText(cutoffHint, stop.value==='' ? 'Leave the cutoff blank to disable it.' :
      delta.value==='' ? '' : 'Restarts at or below '+temperature(stop.valueAsNumber-delta.valueAsNumber,unit.value)+'.');
  }

  function addRow(band={}) {
    const from = heatingNumber(null, band.from, {step:'any','aria-label':'Band starting temperature'});
    const power = heatingNumber(null, band.maxPower ?? 100, {min:1,max:100,required:true,'aria-label':'Maximum burner power (%)'});
    const range = dom('span',{class:'heating-band-range'});
    const bounds = dom('small',{class:'heating-band-bounds'});
    const remove = btn('×',()=>{
      const focus = row.nextElementSibling?.querySelector('button') ?? add;
      row.remove();
      syncBands();
      updateDraft();
      focus.focus();
    },'heating-band-remove',{'aria-label':'Remove power band',title:'Remove power band'});
    const row = dom('div',{class:'heating-band'},
      dom('div',{},range,dom('div',{class:'heating-band-from'},from),bounds),
      dom('div',{class:'heating-band-limit'},power,dom('span',{'aria-hidden':'true'},'%')),
      remove);
    row.controls = {from,power,range,bounds};
    bands.append(row);
    syncBands();
  }

  const add = btn('+ Add band',()=>{
    addRow();
    updateDraft();
    const {from,power} = bands.lastElementChild.controls;
    (from.hidden ? power : from).focus();
  });
  (settings.bands ?? []).forEach(addRow);
  syncBands();
  sensor.onchange = () => { currentSensor=sensor.value; showSensors(data); updateDraft(); };
  unit.onchange = () => {
    if (unit.value===sourceUnit) return;
    const ratio = unit.value==='F' ? 9/5 : 5/9;
    const shift = unit.value==='F' ? 32 : -32*5/9;
    const convert = (input,offset) => {
      if (input.value!=='') input.value=String(Number((input.valueAsNumber*ratio+offset).toFixed(3)));
    };
    convert(stop,shift);
    convert(delta,0);
    Array.from(bands.children).forEach(row=>convert(row.controls.from,shift));
    sourceUnit=unit.value;
    showSensors(data);
    syncBands();
    updateDraft();
  };
  const advanced = dom('details',{class:'heating-advanced'},
    dom('summary',{},'Advanced Settings'),dom('div',{class:'heating-advanced-body'},heatingField('Sensor timeout (seconds)',timeout)));
  advanced.addEventListener('invalid',()=>{advanced.open=true;},true);
  const element = dom('div',{class:'heating-layout'},
    dom('div',{class:'heating-column'},
      heatingPanel('Outdoor Cutoff','Overrides thermostat demand when it’s warm enough outside.',null,
        dom('div',{class:'heating-field-pair heating-sensor-fields'},heatingField('Outdoor sensor',sensor),heatingField('Units',unit)),
        dom('div',{class:'heating-field-pair heating-cutoff-fields'},heatingField('Stop at or above',stop),heatingField('Restart differential',delta)),
        cutoffHint),
      advanced),
    heatingPanel('Maximum-Power Bands','Set the burner limit for each outdoor temperature range.',add,emptyBands,bands));
  element.addEventListener('input',syncBands);
  return {element,show:showSensors,draft:()=>({
    outdoorSensor:currentSensor,unit:unit.value,stopAt:stop.value===''?null:stop.valueAsNumber,
    restartDelta:delta.valueAsNumber,sensorTimeoutSeconds:timeout.valueAsNumber,
    bands:Array.from(bands.children).map(({controls},index)=>({
      from:index===0?null:controls.from.valueAsNumber,maxPower:controls.power.valueAsNumber,
    })),
  })};
}

function zonePumpsPanel() {
  const select = id=>dom('select',{id,class:'field-input'});
  const zone = select('pump-zone');
  const temperature = select('pump-temperature');
  const flow = select('pump-flow');
  const live = dom('p',{class:'heating-hint'});
  const message = dom('p',{class:'heating-error',role:'alert'});
  const view = dom('a',{class:'secondary',hidden:true},'View pump chart');
  const save = btn('Save assignment',persist,'secondary',{disabled:true});
  const controls = dom('div',{class:'grid'},
    heatingField('Heating zone',zone),
    dom('div',{class:'heating-field-pair'},heatingField('Media temperature',temperature),heatingField('Flow rate',flow)),
    live,dom('div',{class:'toolbar'},save,view),message);
  const element = heatingPanel('Zone Pumps','Link pump readings to the thermostat for that loop.',null,controls);
  let assignments;
  let optionsKey;
  let inputs = [];
  let selectedZones = [];
  let busy = false;

  function options(control, entries, placeholder, selected) {
    control.replaceChildren(dom('option',{value:''},placeholder),
      ...entries.map(([value,label])=>dom('option',{value,selected:value===selected},label)));
    control.value = selected;
  }

  function refresh(reset=false, preferredZone=zone.value) {
    const signals = Object.entries(status.haSignals ?? {}).sort(([,a],[,b])=>a.name.localeCompare(b.name));
    const zones = new Map(inputs.map(input=>[input.id,input.name]));
    for (const id of [...selectedZones,...(assignments?.keys() ?? [])]) {
      if (!zones.has(id)) zones.set(id,id);
    }
    const currentZone = zones.has(preferredZone) ? preferredZone : '';
    const saved = assignments?.get(currentZone);
    reset = reset || currentZone!==zone.value;
    const key = JSON.stringify([Array.from(zones),signals]);
    if (reset || key!==optionsKey) {
      optionsKey=key;
      options(zone,Array.from(zones),'Choose a zone',currentZone);
      for (const [control,unit,previous] of [[temperature,'°C',saved?.temperature],[flow,'m³/h',saved?.flow]]) {
        const entries = signals.filter(([,label])=>label.unit===unit).map(([id,label])=>[id,label.name]);
        const value = (reset?previous:control.value) ?? '';
        if (value&&!entries.some(([id])=>id===value)) entries.push([value,titleSignal(value)]);
        options(control,entries,'None',value);
      }
    }
    zone.disabled=busy||!assignments||!zones.size;
    flow.disabled=temperature.disabled=busy||!assignments||!zone.value;
    save.disabled=busy||!assignments||!zone.value||
      (temperature.value===(saved?.temperature ?? '')&&flow.value===(saved?.flow ?? ''));
    view.hidden=!saved;
    if (saved) {
      const query=new URLSearchParams({zone:saved.zone});
      for (const series of ['operating_data.boiler_temp','operating_data.power_pct',saved.temperature,saved.flow].filter(Boolean)) {
        query.append('series',series);
      }
      view.href='#charts/recorded?'+query;
    }
    const thermostat=inputs.find(input=>input.id===zone.value);
    const action=thermostat?.action;
    const parts=[action?('Thermostat · '+action):null];
    for (const id of [saved?.temperature,saved?.flow].filter(Boolean)) {
      const value=signal(id);
      parts.push(titleSignal(id)+' · '+(value==null?'Waiting for reading':withUnit(value,unitSignal(id))));
    }
    const text=parts.filter(Boolean).join(' · ') || (!signals.length?'Waiting for Home Assistant pump sensors.':'');
    if (live.textContent!==text) live.textContent=text;
  }

  async function persist() {
    const pump={zone:zone.value,temperature:temperature.value,flow:flow.value};
    busy=true;
    message.textContent='';
    refresh();
    try {
      await api('external-heating/pumps',{method:'PUT',body:JSON.stringify(pump)});
      if (pump.temperature || pump.flow) assignments.set(pump.zone,pump);
      else assignments.delete(pump.zone);
      toast('Pump assignment saved.');
    } catch (error) {
      message.textContent=error.message;
    } finally {
      busy=false;
      refresh();
    }
  }

  zone.onchange=()=>refresh(true);
  temperature.onchange=flow.onchange=()=>refresh();
  api('external-heating/pumps').then(pumps=>{
    assignments=new Map(pumps.map(pump=>[pump.zone,pump]));
    refresh(true,zone.value || pumps[0]?.zone || '');
  }).catch(error=>{message.textContent=error.message;});
  return {element,show(nextInputs,nextSelectedZones) {
    inputs=nextInputs;
    selectedZones=nextSelectedZones;
    refresh();
  }};
}
