'use strict';

async function externalHeatingPage(token) {
  const title = heading('External heating', 'Manage thermostat demand, the outdoor cutoff, and maximum burner power.');
  main.replaceChildren(title, card('Heating settings', null, empty('Loading settings…')));
  try {
    const data = await api('external-heating');
    if (token === generation) externalHeatingForm(title, data, token);
  } catch (error) {
    if (token === generation) {
      main.replaceChildren(title, card('Heating settings', null, empty(error.message),
        btn('Try again', () => externalHeatingPage(token))));
    }
  }
}

function externalHeatingForm(title, data, token) {
  const settings = data.settings;
  let selected = new Set(settings.thermostats ?? []);
  let currentSensor = settings.outdoorSensor;
  let sourceUnit = settings.unit;
  let inputsKey;
  let saving = false;
  const field = (label, control, extraClass='') => dom('div', {class:'heating-field '+extraClass},
    dom('label', {for:control.id}, label), control);
  const numericInput = (id, value, attrs={}) => dom('input', {id,type:'number',class:'field-input',value,...attrs});
  const panel = (label, description, action, ...children) => dom('section', {class:'heating-panel'},
    dom('header', {class:'heating-panel-head'}, dom('div', {},dom('h2', {},label), description&&dom('p',{},description)),action),children);

  const enabled = dom('input', {id:'heating-enabled',type:'checkbox',role:'switch',class:'heating-switch',
    checked:settings.enabled,disabled:!data.writable&&!settings.enabled});
  const prefix = dom('input', {id:'heating-prefix',class:'field-input',type:'text',value:settings.discoveryPrefix,maxlength:128,required:true});
  const grace = numericInput('heating-grace', settings.dropoutSeconds, {min:5,max:86400,required:true});
  const cycle = numericInput('heating-cycle', settings.minCycleSeconds, {min:30,max:86400,required:true});
  const timeout = numericInput('outdoor-timeout', settings.sensorTimeoutSeconds, {min:10,max:86400,required:true});
  const unit = dom('select', {id:'heating-unit',class:'field-input'},
    dom('option', {value:'C',selected:sourceUnit==='C'}, '°C'),
    dom('option', {value:'F',selected:sourceUnit==='F'}, '°F'));
  const stop = numericInput('outdoor-stop', settings.stopAt, {step:'any',placeholder:'Off','aria-describedby':'cutoff-hint'});
  const delta = numericInput('outdoor-delta', settings.restartDelta, {min:0.01,max:50,step:'any',required:true});
  const sensor = dom('select', {id:'heating-sensor',class:'field-input'});
  const cutoffHint = dom('p', {id:'cutoff-hint',class:'heating-hint'});

  const liveState = dom('span', {class:'heating-state'});
  const report = dom('p', {role:'status',class:'heating-decision'});
  const resume = btn('Resume automatic control',async()=>{
    if (!await confirmChange('Resume automatic control?',
      'The boiler can start if a thermostat is calling for heat.', 'Resume control')) return;
    resume.disabled=true;
    try {
      const next=await api('external-heating/resume',{method:'POST'});
      if (token!==generation) return;
      data=next;
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
  const notices = dom('div', {class:'heating-notices',role:'status'});
  const list = dom('div', {class:'heating-device-list'});
  const count = dom('span', {class:'heating-selected'});
  const feedback = dom('p', {role:'alert',class:'heating-error'});
  const bands = dom('div', {class:'heating-bands'});
  const emptyBands = dom('p', {class:'heating-band-empty'}, 'Using the controller’s maximum burner power.');
  const metrics = [
    ['Heat demand', demandReading], ['Outdoor temperature', outdoorReading], ['Target maximum power', powerReading],
  ].map(([label,value])=>dom('div',{},dom('dt',{},label),value));

  function temperature(value, from=data.settings.unit) {
    if (value == null) return '—';
    if (from!==unit.value) value = unit.value==='F' ? value*9/5+32 : (value-32)*5/9;
    return decimalTenths.format(value)+' °'+unit.value;
  }

  function setText(element, text) {
    if (element.textContent!==text) element.textContent=text;
  }

  function show(next) {
    const inputs = next.inputs ?? [];
    const discovered = new Map(inputs.map(input=>[input.id,input]));
    const thermostatInputs = inputs.filter(input=>input.kind==='thermostat');
    const sensors = inputs.filter(input=>input.kind==='temperature');
    const saved = next.settings.thermostats ?? [];
    const reporting = saved.filter(id=>discovered.get(id)?.action);
    const heating = reporting.filter(id=>discovered.get(id).action==='heating').length;
    const readout = discovered.get(currentSensor)?.temperature ?? (currentSensor && currentSensor===next.settings.outdoorSensor ? next.outdoorTemperature : null);
    const automatic = next.settings.enabled&&next.writable;
    const held = automatic&&next.manualOff;
    liveState.dataset.state = held ? 'held' : automatic&&next.cutoffActive ? 'cutoff' : automatic ? 'active' : 'off';
    setText(liveState, held ? 'Manual stop' : automatic&&next.cutoffActive ? 'Outdoor cutoff active' : automatic ? 'Control active' : 'Control off');
    resume.hidden = !held;
    setText(report, next.settings.enabled ? next.decision : '');
    setText(demandReading, saved.length ? (reporting.length ? heating+' of '+saved.length+' calling' : 'Waiting for state') : 'No thermostats selected');
    setText(outdoorReading, currentSensor ? (readout == null ? 'Waiting for reading' : temperature(readout)) : 'No sensor selected');
    setText(powerReading, next.bandPower == null ? '—' : next.bandPower+'%');
    setText(notices, [next.error,next.powerError].filter(Boolean).join(' '));
    setText(count, selected.size+' selected');

    // Readings update in place so a refresh cannot move keyboard focus.
    const key = JSON.stringify(inputs.map(({action,availability,temperature,...device})=>device));
    if (key!==inputsKey) {
      inputsKey = key;
      const missing = Array.from(selected).filter(id=>!discovered.has(id));
      list.replaceChildren(...[...thermostatInputs,...missing.map(id=>({id,name:id,problem:'Waiting for discovery'}))].map(input=>{
        const source = input.id.startsWith('ha:') ? 'Home Assistant' : 'MQTT';
        const box = dom('input', {type:'checkbox',class:'heating-device-check',checked:selected.has(input.id),
          disabled:Boolean(input.problem&&!selected.has(input.id)),
          'aria-label':input.name,
          onchange:() => {
            if (box.checked) selected.add(input.id); else selected.delete(input.id);
            if (input.problem&&!box.checked) box.disabled=true;
            show(data);
            updateDraft();
          }});
        return dom('label', {class:'heating-device','data-device':input.id,title:source+' · '+input.id},box,
          dom('span', {class:'heating-device-label'},dom('strong',{},input.name),
            dom('span',{class:'heating-device-state','data-reading':'action'},input.problem)),
          dom('span',{class:'heating-device-temp','data-reading':'temperature','aria-label':'Room temperature'}));
      }));
      if (!list.children.length) list.append(empty('No thermostats available yet.'));
      sensor.replaceChildren(dom('option', {value:''}, 'No outdoor sensor'),
        ...sensors.map(input=>dom('option',{value:input.id,disabled:Boolean(input.problem&&input.id!==currentSensor)},input.name)),
        currentSensor&&!discovered.has(currentSensor)&&dom('option',{value:currentSensor},currentSensor+' · Waiting for discovery'));
      sensor.value = currentSensor;
    }
    list.querySelectorAll('[data-device]').forEach(row=>{
      const input = discovered.get(row.dataset.device);
      const state = row.querySelector('[data-reading=action]');
      const value = input?.action;
      state.dataset.state = value==='heating' ? 'heating' : input?.availability==='offline' ? 'offline' : '';
      setText(state, input?.problem ?? (!input ? 'Waiting for discovery' :
        value ? value.charAt(0).toUpperCase()+value.slice(1) : input.availability==='offline' ? 'Offline' : 'Waiting for state'));
      setText(row.querySelector('[data-reading=temperature]'), temperature(input?.temperature));
    });
    for (const option of sensor.options) {
      const input = discovered.get(option.value);
      if (input) setText(option, input.name+(input.temperature!=null?' · '+temperature(input.temperature):'')+(input.problem?' · '+input.problem:''));
    }
  }

  function syncBands() {
    const rows = Array.from(bands.children);
    for (const [index,row] of rows.entries()) {
      const {from,range,bounds} = row.controls;
      const next = rows[index+1]?.controls.from;
      from.hidden = index===0;
      from.required = index!==0;
      setText(range, index!==0 ? 'From (°'+unit.value+')' : rows.length===1 ? 'All temperatures' : 'Coldest band');
      setText(bounds, next&&next.value!=='' ? 'Below '+temperature(next.valueAsNumber,unit.value) :
        index!==0&&index===rows.length-1 ? 'And above' : '');
      row.classList.toggle('heating-band-base', index===0);
    }
    emptyBands.hidden = rows.length!==0;
    add.disabled = rows.length>=16;
    setText(cutoffHint, stop.value==='' ? 'Leave the cutoff blank to disable it.' :
      delta.value==='' ? '' : 'Restarts at or below '+temperature(stop.valueAsNumber-delta.valueAsNumber,unit.value)+'.');
  }

  function addRow(band={}) {
    const from = numericInput(null, band.from, {step:'any','aria-label':'Band starting temperature'});
    const power = numericInput(null, band.maxPower ?? 100, {min:1,max:100,required:true,'aria-label':'Maximum burner power (%)'});
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
  sensor.onchange = () => { currentSensor=sensor.value; show(data); updateDraft(); };
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
    show(data);
    syncBands();
    updateDraft();
  };
  const clearSources = btn('Clear device selections',()=>{
    selected=new Set();
    currentSensor='';
    inputsKey=undefined;
    show(data);
    updateDraft();
  },'heating-quiet');
  const save = dom('button',{type:'submit',class:'primary'},'Save changes');
  const discard = btn('Discard',()=>externalHeatingForm(title,data,token),'heating-quiet');
  const draftState = dom('span',{class:'heating-draft'});
  const saveDock = dom('div',{class:'heating-save'},
    dom('div',{},draftState,feedback),dom('div',{class:'heating-save-actions'},discard,save));
  const advanced = dom('details',{class:'heating-advanced'},
    dom('summary',{},'Advanced settings'), dom('div',{class:'heating-advanced-body'},
      dom('div',{class:'heating-field-pair'},field('Dropout grace (seconds)',grace),field('Minimum cycle (seconds)',cycle)),
      field('Sensor timeout (seconds)',timeout),
      field('MQTT discovery prefix',prefix),clearSources));

  function draft() {
    return {
      enabled:enabled.checked, discoveryPrefix:prefix.value, thermostats:Array.from(selected),
      dropoutSeconds:grace.valueAsNumber, minCycleSeconds:cycle.valueAsNumber,
      outdoorSensor:currentSensor, unit:unit.value, stopAt:stop.value===''?null:stop.valueAsNumber,
      restartDelta:delta.valueAsNumber,sensorTimeoutSeconds:timeout.valueAsNumber,
      bands:Array.from(bands.children).map(({controls},index)=>({
        from:index===0?null:controls.from.valueAsNumber,maxPower:controls.power.valueAsNumber,
      })),
    };
  }
  const baseline = JSON.stringify(draft());
  function updateDraft() {
    const dirty = JSON.stringify(draft())!==baseline;
    save.disabled = saving||!dirty;
    discard.hidden = !dirty;
    saveDock.dataset.dirty = String(dirty);
    setText(draftState, dirty ? 'Unsaved changes' : 'Settings saved');
  }

  async function submit(event) {
    event.preventDefault();
    if (saving) return;
    const proposed=draft();
    if (proposed.discoveryPrefix!==settings.discoveryPrefix && (selected.size||currentSensor)) {
      advanced.open=true;
      setText(feedback,'Clear device selections and remove the outdoor cutoff and bands before changing the discovery prefix.');
      return;
    }
    saving=true;
    updateDraft();
    setText(feedback,'');
    try {
      if (proposed.enabled && !await confirmChange('Apply external heating settings?',
        'Boiler Mate can start or stop the boiler and change its maximum burner power using these settings.', 'Save changes')) return;
      const next=await api('external-heating',{method:'PUT',body:JSON.stringify(proposed)});
      if (token!==generation) return;
      toast('External heating settings saved.');
      externalHeatingForm(title,next,token);
    } catch (error) {
      setText(feedback,error.message);
    } finally {
      saving=false;
      updateDraft();
    }
  }

  const form = dom('form',{class:'heating-page',onsubmit:submit},
    dom('section',{class:'heating-summary','aria-label':'External heating status'},
      dom('div',{class:'heating-summary-top'},
        dom('div',{},liveState,report),
        dom('div',{class:'heating-summary-actions'},resume,
          dom('label',{for:enabled.id,class:'heating-enable'},'Enable external control',enabled))),
      !data.writable&&dom('p',{class:'heating-readonly'},'Boiler writes and a controller PIN are required to enable control.'),
      dom('dl',{class:'heating-live'},metrics)),
    notices,
    dom('div',{class:'heating-layout'},
      dom('div',{class:'heating-column'},
        panel('Thermostats','Select every room that should be able to call for heat.',count,list),
        advanced),
      dom('div',{class:'heating-column'},
        panel('Outdoor cutoff','Overrides thermostat demand when it’s warm enough outside.',null,
          dom('div',{class:'heating-field-pair heating-sensor-fields'},field('Outdoor sensor',sensor),field('Units',unit)),
          dom('div',{class:'heating-field-pair heating-cutoff-fields'},field('Stop at or above',stop),field('Restart differential',delta)),
          cutoffHint),
        panel('Maximum-power bands','Set the burner limit for each outdoor temperature range.',add,emptyBands,bands))),
    saveDock);
  form.addEventListener('input',()=>{ syncBands(); updateDraft(); });
  form.addEventListener('invalid',event=>{ if (advanced.contains(event.target)) advanced.open=true; },true);
  main.replaceChildren(title,form);
  show(data);
  updateDraft();

  async function refreshHeating() {
    if (!form.isConnected) return;
    try {
      const next=await api('external-heating');
      if (!form.isConnected) return;
      data=next;
      show(next);
    } catch (error) {
      if (form.isConnected) setText(report,error.message);
    }
    if (form.isConnected) setTimeout(refreshHeating,3000);
  }
  setTimeout(refreshHeating,3000);
}
