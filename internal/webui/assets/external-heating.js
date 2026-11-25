'use strict';

async function externalHeatingPage(token) {
  const title = heading('External heating', 'Use MQTT thermostats and an outdoor sensor to control the boiler.');
  main.replaceChildren(title, card('External heating control', null, empty('Loading settings…')));
  try {
    const data = await api('external-heating');
    if (token === generation) externalHeatingForm(title, data, token);
  } catch (error) {
    if (token === generation) {
      main.replaceChildren(title, card('External heating control', null, empty(error.message),
        btn('Try again', () => externalHeatingPage(token))));
    }
  }
}

function externalHeatingForm(title, data, token) {
  const settings = data.settings;
  let selected = new Set(settings.thermostats ?? []);
  let sourceUnit = settings.unit;
  let inputsKey;
  let saving = false;
  const name = (id, text, control) => dom('div', {},
    dom('label', {for:id,class:'field-name'}, text), control);
  const numericInput = (id, value, attrs={}) => dom('input', {id,type:'number',class:'field-input',value,...attrs});
  const enabled = dom('input', {id:'heating-enabled',type:'checkbox',checked:settings.enabled,disabled:!data.writable&&!settings.enabled});
  const prefix = dom('input', {id:'heating-prefix',class:'field-input',type:'text',value:settings.discoveryPrefix,maxlength:128,required:true});
  const grace = numericInput('heating-grace', settings.dropoutSeconds, {min:5,max:86400,required:true});
  const cycle = numericInput('heating-cycle', settings.minCycleSeconds, {min:30,max:86400,required:true});
  const timeout = numericInput('outdoor-timeout', settings.sensorTimeoutSeconds, {min:10,max:86400,required:true});
  const unit = dom('select', {id:'heating-unit',class:'field-input'},
    dom('option', {value:'C',selected:sourceUnit==='C'}, 'Celsius (°C)'),
    dom('option', {value:'F',selected:sourceUnit==='F'}, 'Fahrenheit (°F)'));
  const stop = numericInput('outdoor-stop', settings.stopAt, {step:'any',placeholder:'Disabled'});
  const delta = numericInput('outdoor-delta', settings.restartDelta, {min:0.01,max:50,step:'any',required:true});
  const sensor = dom('select', {id:'heating-sensor',class:'field-input'});
  sensor.value = '';
  const list = dom('div', {class:'stack'});
  const report = dom('p', {role:'status'});
  const discovered = dom('p', {class:'muted'});
  const notices = dom('div', {class:'stack'});
  const feedback = dom('p', {role:'alert',class:'muted'});
  const bands = dom('tbody');
  let currentSensor = settings.outdoorSensor;
  sensor.onchange = () => { currentSensor = sensor.value; };

  function show(next) {
    const info = [next.decision];
    if (next.outdoorTemperature != null) info.push('Outdoors '+display(next.outdoorTemperature)+' °'+next.settings.unit);
    if (next.bandPower != null) info.push('Target maximum '+next.bandPower+'%');
    report.textContent = info.join(' · ');
    notices.replaceChildren(...[next.error,next.powerError].filter(Boolean).map(text => dom('p', {class:'muted'}, text)));
    const inputs = next.inputs ?? [];
    // Keep focused controls in place when only a thermostat's action changes.
    const key = JSON.stringify(inputs.map(({action,...input}) => input));
    if (key !== inputsKey) {
      inputsKey = key;
      const thermostatInputs = inputs.filter(input => input.kind==='thermostat');
      const absent = Array.from(selected).filter(id => !thermostatInputs.some(input => input.id===id));
      list.replaceChildren(...[...thermostatInputs,...absent.map(id => ({id,name:id,problem:'Waiting for discovery.'}))].map(input => {
        const box = dom('input', {type:'checkbox',checked:selected.has(input.id),disabled:input.problem&&!selected.has(input.id),
          onchange:() => { if (box.checked) selected.add(input.id); else selected.delete(input.id); }});
        const state = dom('span', {class:'muted','data-heating-action':input.id});
        return dom('label', {class:'flex items-start gap-2'}, box, dom('span', {class:'min-w-0'},
          dom('span', {class:'field-name'}, input.name), dom('small', {class:'field-id'}, input.id),
          input.problem ? dom('small', {class:'muted'}, input.problem) : state));
      }));
      if (!list.children.length) list.append(empty('No MQTT thermostats discovered yet.'));
      const sensors = inputs.filter(input => input.kind==='temperature');
      sensor.replaceChildren(dom('option', {value:''}, 'No outdoor sensor'),
        ...sensors.map(input => dom('option', {value:input.id,disabled:input.problem&&input.id!==currentSensor},
          input.name+(input.problem?' · '+input.problem:''))),
        currentSensor&&!sensors.some(input=>input.id===currentSensor) &&
          dom('option', {value:currentSensor}, currentSensor+' · Waiting for discovery'));
      sensor.value = currentSensor;
      discovered.textContent = sensors.length ? '' : 'No MQTT temperature sensors discovered yet.';
    }
    const states = new Map(inputs.map(input=>[input.id,input.action]));
    list.querySelectorAll('[data-heating-action]').forEach(element => {
      const action = states.get(element.dataset.heatingAction);
      element.textContent = action ? action.charAt(0).toUpperCase()+action.slice(1) :
        selected.has(element.dataset.heatingAction) ? 'Waiting for thermostat action' : '';
    });
  }

  const maximumLabel = 'Maximum burner power (%)';
  const fromLabel = dom('th', {scope:'col'});
  const syncBands = () => {
    fromLabel.textContent = 'From (°'+unit.value+')';
    Array.from(bands.rows).forEach((row,index) => {
      const first = index===0;
      row.controls.from.hidden = first;
      row.controls.from.required = !first;
      row.controls.lowest.hidden = !first;
    });
    add.disabled = bands.rows.length>=16;
  };
  const addRow = (band={}) => {
    const from = numericInput(null, band.from, {step:'any','aria-label':'Starting temperature'});
    const power = numericInput(null, band.maxPower ?? 100, {min:1,max:100,required:true,'aria-label':maximumLabel});
    const lowest = dom('span', {}, 'Coldest band');
    const remove = btn('Remove', () => { row.remove(); syncBands(); }, 'secondary');
    const row = dom('tr', {}, dom('td', {}, lowest,from),dom('td', {},power),dom('td', {},remove));
    row.controls = {from,power,lowest};
    bands.append(row);
    syncBands();
  };
  const add = btn('Add power band', () => {
    addRow();
    const {from,power} = bands.lastElementChild.controls;
    (from.hidden ? power : from).focus();
  });
  (settings.bands ?? []).forEach(addRow);
  syncBands();
  unit.onchange = () => {
    const convertingToF = unit.value==='F';
    if (unit.value===sourceUnit) return;
    const ratio = convertingToF ? 9/5 : 5/9;
    const shift = convertingToF ? 32 : -32*5/9;
    const convert = (input, offset) => {
      if (input.value !== '') input.value = String(Number((input.valueAsNumber*ratio+offset).toFixed(3)));
    };
    convert(stop, shift);
    convert(delta, 0);
    Array.from(bands.rows).forEach(row => convert(row.controls.from, shift));
    sourceUnit = unit.value;
    syncBands();
  };
  const clearSources = btn('Clear device selections', () => {
    selected = new Set();
    currentSensor = '';
    inputsKey = undefined;
    show(data);
  });
  const save = dom('button', {type:'submit',class:'primary'}, 'Save settings');
  const submit = async event => {
    event.preventDefault();
    if (saving) return;
    if (prefix.value !== settings.discoveryPrefix && (selected.size || currentSensor)) {
      feedback.textContent = 'Clear device selections and remove the outdoor cutoff and bands before changing the discovery prefix.';
      return;
    }
    const proposed = {
      enabled:enabled.checked, discoveryPrefix:prefix.value, thermostats:Array.from(selected),
      dropoutSeconds:grace.valueAsNumber, minCycleSeconds:cycle.valueAsNumber,
      outdoorSensor:currentSensor, unit:unit.value, stopAt:stop.value===''?null:stop.valueAsNumber,
      restartDelta:delta.valueAsNumber,sensorTimeoutSeconds:timeout.valueAsNumber,
      bands:Array.from(bands.rows).map(({controls}, index)=>({
        from:index===0?null:controls.from.valueAsNumber,maxPower:controls.power.valueAsNumber,
      })),
    };
    saving = true;
    save.disabled = true;
    feedback.textContent = '';
    try {
      if (proposed.enabled && !await confirmChange('Apply external heating settings?',
        'Boiler Mate can start or stop the boiler and change its maximum burner power using these settings.', 'Save settings')) return;
      const next = await api('external-heating', {method:'PUT',body:JSON.stringify(proposed)});
      if (token !== generation) return;
      toast('External heating settings saved.');
      externalHeatingForm(title, next, token);
    } catch (error) {
      feedback.textContent = error.message;
    } finally {
      saving = false;
      save.disabled = false;
    }
  };
  const layout = {class:'grid gap-3 sm:grid-cols-2'};
  main.replaceChildren(title,
    dom('form', {class:'stack',onsubmit:submit},
      card('Control status', null, report, notices,
        dom('label', {class:'flex items-center gap-2 mt-3'}, enabled, 'Enable external heating control'),
        !data.writable&&dom('p', {class:'muted'}, 'Enable boiler writes and configure a controller PIN to use external control.')),
      card('Thermostats', 'Any selected thermostat can call for heat. The boiler stops when demand ends.',
        list, dom('div', {...layout,class:layout.class+' mt-3'},
          name(grace.id,'Dropout grace (seconds)',grace),name(cycle.id,'Minimum cycle interval (seconds)',cycle))),
      card('Outdoor cutoff', 'At the cutoff temperature, the outdoor sensor overrides thermostat demand.',
        dom('div', layout, name(sensor.id,'Temperature sensor',sensor),name(unit.id,'Temperature units',unit)),
        discovered, dom('div', {...layout,class:layout.class+' mt-3'},
          name(stop.id,'Stop at or above',stop),name(delta.id,'Restart differential',delta),
          name(timeout.id,'Sensor timeout (seconds)',timeout))),
      card('Maximum-power bands', 'The first band covers the coldest temperatures. Each additional band starts at the temperature you enter.',
        dom('div', {class:'table-scroll'},dom('table', {},dom('thead', {},dom('tr', {},
          fromLabel,dom('th', {scope:'col'},maximumLabel),dom('th', {scope:'col'},'Actions'))),bands)),add),
      card('MQTT discovery', null, name(prefix.id,'Discovery prefix',prefix),dom('div', {class:'toolbar mt-3'},clearSources)),
      dom('div', {class:'toolbar'},save,feedback)));
  show(data);

  async function refreshHeating() {
    if (!report.isConnected) return;
    try {
      const next = await api('external-heating');
      if (!report.isConnected) return;
      data = next;
      show(next);
    } catch (error) {
      if (report.isConnected) report.textContent = error.message;
    }
    if (report.isConnected) setTimeout(refreshHeating, 3000);
  }
  setTimeout(refreshHeating, 3000);
}
