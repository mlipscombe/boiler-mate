'use strict';

async function externalHeatingPage(pageID, token) {
  const zones = pageID==='zones';
  const label = zones ? 'Zones' : 'Outdoor Climate';
  const title = heading(label, zones ? 'Set which zones can call for heat and which pump readings to track.' :
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
  const controlKey = zones ? 'zoneControl' : 'outdoorControl';
  const controlName = zones ? 'Zone control' : 'Outdoor climate control';
  let savedEnabled = data.settings[controlKey];
  let saving = false;
  let resuming = false;
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
    resuming=true;
    updateDraft();
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
      resuming=false;
      updateDraft();
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
    if (enabled.checked!==savedEnabled) patch[controlKey]=enabled.checked;
    return patch;
  }

  function updateDraft() {
    const dirty = Object.keys(changes()).length>0 || Boolean(section.pumpChanges?.().length);
    save.disabled = saving||resuming||!dirty;
    discard.disabled = saving||resuming;
    discard.hidden = !dirty;
    resume.disabled = saving||resuming;
    fields.disabled = saving;
    enabled.disabled = saving||(!data.writable&&!savedEnabled);
    saveDock.dataset.dirty = String(dirty);
    setHeatingText(draftState,dirty ? 'Unsaved changes' : 'Settings saved');
  }

  function show(next) {
    data=next;
    if (enabled.checked===savedEnabled) enabled.checked=next.settings[controlKey];
    savedEnabled=next.settings[controlKey];
    readOnly.hidden=next.writable;
    const automatic = savedEnabled&&next.writable;
    const held = automatic&&next.manualOff;
    liveState.dataset.state = held ? 'held' : automatic&&next.cutoffActive ? 'cutoff' : automatic ? 'active' : 'off';
    setHeatingText(liveState,held ? controlName+' paused' : automatic ? controlName+' on' :
      savedEnabled ? controlName+' unavailable' : controlName+' off');
    resume.hidden = !held;
    setHeatingText(report,savedEnabled ? next.decision : '');
    const selected = next.settings.thermostats ?? [];
    const reporting = (next.inputs ?? []).filter(input=>input.action&&selected.includes(input.id));
    const calling = reporting.filter(input=>input.action==='heating').length;
    setHeatingText(demandReading,selected.length ?
      (reporting.length ? calling+' of '+selected.length+' calling' : 'Waiting for state') : 'No thermostats selected');
    setHeatingText(outdoorReading,next.settings.outdoorSensor ?
      (next.outdoorTemperature!=null ? heatingTemperature(next.outdoorTemperature,next.settings.unit) :
        next.lastOutdoorReading ? heatingTemperature(next.lastOutdoorReading.celsius,'C',next.settings.unit)+' (last seen)' : 'Waiting for reading') : 'No sensor selected');
    setHeatingText(powerReading,next.bandPower==null ? '—' : next.bandPower+'%');
    setHeatingText(notices,[next.error,next.powerError].filter(Boolean).join(' '));
    section.show(next);
    updateDraft();
  }

  async function submit(event) {
    event.preventDefault();
    const proposed=changes();
    const heatingChanged=Object.keys(proposed).length>0;
    const pumpChanges=section.pumpChanges?.() ?? [];
    if (saving||resuming||(!heatingChanged&&!pumpChanges.length)) return;
    saving=true;
    updateDraft();
    setHeatingText(feedback,'');
    let pumpsSaved=0;
    try {
      if (heatingChanged && (proposed[controlKey] ?? savedEnabled) && !await confirmChange('Apply '+controlName.toLowerCase()+' settings?',
        zones ? 'Boiler Mate can start or stop the boiler based on thermostat demand.' :
          'Boiler Mate can stop or resume the boiler at the outdoor cutoff and change its maximum burner power.', 'Save changes')) return;
      if (token!==generation) return;
      // A selected thermostat can be used for a pump assignment even while it's undiscovered.
      for (const pump of pumpChanges) {
        await section.savePump(pump);
        pumpsSaved++;
      }
      const next=heatingChanged ? await api('external-heating',{method:'PATCH',body:JSON.stringify(proposed)}) : data;
      if (token!==generation) return;
      toast(label+' settings saved.');
      externalHeatingForm(title,label,zones,next,token);
    } catch (error) {
      const prefix=pumpsSaved ? (pumpsSaved===1 ? 'One pump assignment saved. ' : pumpsSaved+' pump assignments saved. ') : '';
      setHeatingText(feedback,prefix+error.message);
    } finally {
      saving=false;
      updateDraft();
    }
  }

  const fields = dom('fieldset',{class:'heating-page'},section.element);
  const form = dom('form',{class:'heating-page',onsubmit:submit},
    dom('section',{class:'heating-summary','aria-label':controlName+' status'},
      dom('div',{class:'heating-summary-top'},
        dom('div',{},liveState,report,
          dom('p',{id:'heating-control-scope',class:'heating-decision'},zones ?
            'Uses thermostat demand to start and stop the boiler.' : 'Applies the outdoor cutoff and maximum-power bands.')),
        dom('div',{class:'heating-summary-actions'},resume,
          dom('label',{for:enabled.id,class:'heating-enable'},'Enable '+controlName.toLowerCase(),enabled))),
      readOnly,dom('dl',{class:'heating-live'},metrics)),
    notices,fields,saveDock);
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
  const cards = new Map();
  let assignments;
  let cardNumber = 0;
  const prefix = dom('input',{id:'heating-prefix',class:'field-input',type:'text',value:settings.discoveryPrefix,maxlength:128,required:true});
  const grace = heatingNumber('heating-grace',settings.dropoutSeconds,{min:5,max:86400,required:true});
  const cycle = heatingNumber('heating-cycle',settings.minCycleSeconds,{min:30,max:86400,required:true});
  const list = dom('div',{class:'heating-zones'});
  const noZones = empty('No thermostats available yet.');
  const pumpError = dom('p',{class:'heating-error',role:'alert'});
  const retry = btn('Retry loading pump assignments',loadAssignments,'secondary');
  const loadError = dom('div',{class:'heating-pump-error',hidden:true},pumpError,retry);

  function show(next) {
    data=next;
    const inputs = new Map((next.inputs ?? []).filter(input=>input.kind==='thermostat').map(input=>[input.id,input]));
    const saved = new Set(next.settings.thermostats ?? []);
    const ids = new Set([...inputs.keys(),...saved,...selected,...(assignments?.keys() ?? [])]);
    for (const id of ids) {
      if (cards.has(id)) continue;
      const card = heatingZoneCard(id,++cardNumber,checked=>{
        if (checked) selected.add(id); else selected.delete(id);
        updateDraft();
      });
      cards.set(id,card);
      list.append(card.element);
    }
    const signals = Object.entries(status.haSignals ?? {}).sort(([,a],[,b])=>a.name.localeCompare(b.name));
    const sensors = {
      temperature:signals.filter(([,signal])=>signal.unit==='°C').map(([key,signal])=>[key,signal.name]),
      flow:signals.filter(([,signal])=>signal.unit==='m³/h').map(([key,signal])=>[key,signal.name]),
    };
    for (const [id,card] of cards) {
      const pump = assignments?.get(id);
      if (!ids.has(id)&&!card.pumpChange(pump)) {
        card.element.remove();
        cards.delete(id);
        continue;
      }
      card.show(inputs.get(id),next.settings.unit,selected.has(id),saved.has(id),assignments&&sensors,pump);
    }
    noZones.hidden=cards.size>0;
    list.hidden=cards.size===0;
  }

  async function loadAssignments() {
    retry.disabled=true;
    try {
      const pumps=await api('external-heating/pumps');
      assignments=new Map(pumps.map(pump=>[pump.zone,pump]));
      loadError.hidden=true;
      show(data);
      updateDraft();
    } catch (error) {
      setHeatingText(pumpError,'Could not load pump assignments. '+error.message);
      loadError.hidden=false;
    } finally {
      retry.disabled=false;
    }
  }

  async function savePump(pump) {
    const card = cards.get(pump.zone);
    try {
      const saved = await api('external-heating/pumps',{method:'PUT',body:JSON.stringify(pump)});
      if (saved.temperature||saved.flow) assignments.set(saved.zone,saved);
      else assignments.delete(saved.zone);
      show(data);
    } catch (error) {
      throw new Error('Could not save pump readings for '+card.name()+'. '+error.message);
    }
  }

  const clear = btn('Clear thermostat selections',()=>{
    selected.clear();
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
  const element = dom('div',{class:'heating-page'},loadError,noZones,list,advanced);
  loadAssignments();
  return {element,show,savePump,
    pumpChanges:()=>assignments ? Array.from(cards,([id,card])=>card.pumpChange(assignments.get(id))).filter(Boolean) : [],
    draft:()=>({
      discoveryPrefix:prefix.value,thermostats:Array.from(selected),
      dropoutSeconds:grace.valueAsNumber,minCycleSeconds:cycle.valueAsNumber,
    }),
  };
}

function heatingZoneCard(id, number, setSelected) {
  const title = dom('h2',{id:'zone-name-'+number});
  const action = dom('span',{class:'heating-zone-state'});
  const room = dom('span',{class:'heating-zone-temp','aria-label':'Room temperature'});
  const demand = dom('input',{type:'checkbox',class:'heating-zone-check',onchange:()=>setSelected(demand.checked)});
  const temperature = zonePumpField('pump-temperature-'+number,'Pump temperature');
  const flow = zonePumpField('pump-flow-'+number,'Pump flow');
  const view = dom('a',{class:'secondary heating-zone-chart',hidden:true},'View pump chart');
  const element = dom('section',{class:'heating-panel heating-zone','aria-labelledby':title.id},
    dom('header',{class:'heating-zone-header'},dom('div',{},title,action),room),
    dom('label',{class:'heating-zone-demand'},demand,'Can call for heat'),
    dom('div',{class:'heating-zone-pumps'},temperature.element,flow.element),view);

  function show(input, unit, selected, saved, sensors, pump) {
    const name=input?.name ?? (title.textContent || id);
    const source=id.startsWith('ha:') ? 'Home Assistant' : 'MQTT';
    title.title=source+' · '+id;
    setHeatingText(title,name);
    demand.checked=selected;
    demand.disabled=Boolean((!input||input.problem)&&!selected&&!saved);
    const state=input?.action;
    const offline=input?.availability==='offline';
    action.dataset.state=offline ? 'offline' : state==='heating' ? 'heating' : '';
    setHeatingText(action,input?.problem ?? (!input ? 'Waiting for discovery' : offline ? 'Offline' :
      state==='heating' ? 'Calling for heat' : state ? state[0].toUpperCase()+state.slice(1) : 'Waiting for state'));
    setHeatingText(room,heatingTemperature(input?.temperature,unit));

    if (sensors) {
      temperature.show(sensors.temperature,pump?.temperature ?? '');
      flow.show(sensors.flow,pump?.flow ?? '');
    }
    view.hidden=!pump;
    if (pump) {
      const query=new URLSearchParams({zone:id});
      for (const series of ['operating_data.boiler_temp','operating_data.power_pct',pump.temperature,pump.flow].filter(Boolean)) {
        query.append('series',series);
      }
      view.href='#charts/recorded?'+query;
    }
  }

  function pumpChange(pump) {
    const values = {zone:id,temperature:temperature.value(),flow:flow.value()};
    return values.temperature!==(pump?.temperature ?? '') || values.flow!==(pump?.flow ?? '') ? values : null;
  }

  return {element,show,pumpChange,name:()=>title.textContent};
}

function zonePumpField(id, label) {
  const control = dom('select',{id,class:'field-input',disabled:true,'aria-describedby':id+'-reading'});
  const readout = dom('small',{id:id+'-reading',class:'heating-pump-reading',hidden:true});
  const element = dom('div',{},heatingField(label,control),readout);
  let optionsKey;

  function updateReading() {
    const key=control.value;
    const value=key ? signal(key) : null;
    readout.hidden=!key;
    setHeatingText(readout,key ? (value==null ? 'Waiting for reading' : withUnit(value,unitSignal(key))) : '');
  }

  control.onchange=updateReading;
  function show(entries, saved) {
    const value = optionsKey===undefined ? saved : control.value;
    const unavailable = Array.from(new Set([value,saved])).filter(id=>id&&!entries.some(([key])=>key===id));
    const key = JSON.stringify([entries,unavailable]);
    if (key!==optionsKey) {
      optionsKey=key;
      control.replaceChildren(dom('option',{value:''},'No sensor'),
        ...entries.map(([id,name])=>dom('option',{value:id},name)),
        ...unavailable.map(id=>dom('option',{value:id},titleSignal(id)+' · Unavailable')));
      control.value=value;
    }
    control.disabled=!entries.length&&!unavailable.length;
    updateReading();
  }

  return {element,show,value:()=>control.value};
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
  const sensorReading = dom('p',{class:'heating-hint',role:'status'});
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
        currentSensor&&!discovered.has(currentSensor)&&dom('option',{value:currentSensor},currentSensor));
      sensor.value=currentSensor;
    }
    for (const option of sensor.options) {
      if (!option.value) continue;
      const input = discovered.get(option.value);
      const savedSensor=option.value===next.settings.outdoorSensor;
      const value=input?.temperature ?? (savedSensor ? next.outdoorTemperature : null);
      const last=savedSensor ? next.lastOutdoorReading : null;
      const current=value==null ? null : temperature(value);
      const previous=last ? temperature(last.celsius,'C') : null;
      const missing=input?.problem ?? (input?.availability==='offline' ? 'Offline' : input ? 'Waiting for reading' : 'Waiting for discovery');
      option.disabled=Boolean(input?.problem&&option.value!==currentSensor);
      setHeatingText(option,(input?.name ?? option.value)+' · '+(current ?? (previous ? previous+' (last seen)' : missing)));
      if (option.value===currentSensor) {
        setHeatingText(sensorReading,current ? 'Current reading '+current : previous ?
          'Last seen '+previous+' on '+datetime.format(new Date(last.at))+'. '+missing+'.' : missing+'.');
      }
    }
    sensorReading.hidden=!currentSensor;
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
        sensorReading,
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
