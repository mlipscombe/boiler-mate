'use strict';

function pelletLedgerPage(token) {
  const periods = [['','Current periods'],['hour','Hourly'],['day','Daily'],['week','Weekly'],['month','Monthly'],['year','Yearly']];
  const history = dom('select',{class:'field-input','aria-label':'Cost reporting period',onchange:()=>load(true)},
    periods.map(([value,label])=>dom('option',{value},label)));
  const summary = dom('p',{class:'pellet-stock','aria-live':'polite'});
  const coverage = dom('p',{class:'pellet-coverage muted'});
  const reports = dom('div');
  const brands = dom('div',{},empty('No purchases yet.'));
  const entries = dom('div',{},empty('No purchases yet.'));
  const pageError = dom('div',{role:'status'});
  const purchase = pelletPurchaseForm(savePurchase);
  const settings = pelletSupplyForm(saveSettings);
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  let ledger;
  let request = 0;
  let entriesKey;

  main.replaceChildren(heading('Pellet purchase ledger'),
    renderSections('consumption',consumptionSections,'ledger','Consumption sections'),
    pageError,
    dom('div',{class:'stack'},
      dom('div',{class:'pellet-columns'},
        card('Pellet supply','Estimated stock on hand, including pellets in the hopper.',summary,settings.form),
        card('Record a purchase',null,purchase.form)),
      card('Fuel cost','Costs use the oldest available purchase first.',dom('div',{class:'toolbar'},history),reports,coverage),
      card('By brand','Estimated burn dates and costs for comparison with the performance charts.',brands),
      card('Purchase history',null,entries)));
  refreshPageHistory = () => load(false);
  load(true);

  async function load(showError) {
    const index = ++request;
    const query = new URLSearchParams({period:history.value});
    if(zone)query.set('tz',zone);
    try {
      const next = await api('pellets?'+query);
      if(token!==generation||index!==request)return;
      ledger = next;
      pageError.replaceChildren();
      purchase.ready(ledger.settings.currency);
      settings.ready(ledger.settings,next.purchases.length>0);
      paintReport();
    } catch(error) {
      if(showError&&token===generation&&index===request) {
        pageError.replaceChildren(empty(error.message),btn('Retry ledger',()=>load(true)));
      }
    }
  }

  function paintReport() {
    const money = new Intl.NumberFormat(undefined,{style:'currency',currency:ledger.settings.currency});
    const cost = (used,priced,value) => {
      if(used>0&&priced===0)return 'Unpriced';
      return priced+1e-7<used ? money.format(value)+' (partial)' : money.format(value);
    };
    const kg = value => withUnit(value,'kg');
    const displayUnit = ledger.settings.lowUnit;
    const onHand = displayUnit==='lb' ? ledger.remainingKg/0.45359237 : ledger.remainingKg;
    const low = ledger.settings.lowWeight>0 && ledger.sampledAt && onHand<=ledger.settings.lowWeight;
    summary.textContent = ledger.remainingKg==null ? 'Add pellets to start tracking stock.' :
      withUnit(onHand,displayUnit)+(low?' · Low supply':'');
    const sample = Date.parse(ledger.sampledAt);
    const began = Date.parse(ledger.trackedFrom);
    coverage.textContent = ledger.sampledAt ?
      'Tracking since '+datetime.format(began)+'. Last counter reading '+datetime.format(sample)+
        (Date.now()-sample>300000 ? ' · readings are stale.' : '.') :
      'Waiting for the controller’s pellet counter. Costs start with the first reading.';
    const labels = {hour:'This hour',day:'Today',week:'This week',month:'This month',year:'This year'};
    reports.replaceChildren(pelletTable(['Period','Used','Priced','Cost'],ledger.periods.map(period=>{
      const from = Date.parse(period.from);
      const available = sample>=from && began<Date.parse(period.to);
      const name = history.value ? pelletPeriodLabel(period.kind,from) : labels[period.kind];
      return available ? [name,kg(period.usedKg),kg(period.pricedKg),cost(period.usedKg,period.pricedKg,period.cost)] : [name,'—','—','—'];
    })));
    const dateOnly = new Intl.DateTimeFormat(undefined,{dateStyle:'medium'});
    const burnRange = brand => {
      if(!brand.firstUsed)return null;
      const first = dateOnly.format(brand.firstUsed*1000);
      const last = dateOnly.format((brand.lastUsed-1)*1000);
      return dom('small',{class:'pellet-burn-dates'}, first===last?first:first+' – '+last);
    };
    brands.replaceChildren(ledger.brands.length ? pelletTable(['Brand','Logged','Used','On hand','Cost of use'],ledger.brands.map(brand=>[
      dom('span',{},brand.name||'Unspecified',burnRange(brand)),kg(brand.purchasedKg),kg(brand.usedKg),kg(Math.max(0,brand.purchasedKg-brand.usedKg)),
      cost(brand.usedKg,brand.pricedKg,brand.cost),
    ])) : empty('No purchases yet.'));
    const nextKey = JSON.stringify([ledger.settings.currency,ledger.purchases]);
    if(entriesKey===nextKey)return;
    entriesKey = nextKey;
    entries.replaceChildren(ledger.purchases.length ? pelletTable(['Purchased','Brand','Price basis','Stock added','Total price','Actions'],
      [...ledger.purchases].reverse().map(entry=>[
        datetime.format(new Date(entry.at)),entry.brand||'—',
        display(entry.quantity)+' × '+withUnit(entry.weight,entry.unit)+(entry.price==null?'':' at '+money.format(entry.price)+' each'),
        withUnit(entry.quantity*entry.weight,entry.unit),entry.price==null?'Unpriced':money.format(entry.quantity*entry.price),
        dom('div',{class:'pellet-actions'},btn('Edit',()=>purchase.edit(entry),'secondary',{'aria-label':'Edit purchase from '+datetime.format(new Date(entry.at))}),
          btn('Delete',()=>removePurchase(entry),'secondary',{'aria-label':'Delete purchase from '+datetime.format(new Date(entry.at))})),
      ])) : empty('No purchases yet.'));
  }

  async function savePurchase(id,entry) {
    await api('pellets/purchases/'+id,{method:'PUT',body:JSON.stringify(entry)});
    if(token===generation) {
      toast('Pellet purchase saved.');
      await load(true);
    }
  }

  async function removePurchase(entry) {
    if(!await confirmChange('Delete pellet purchase?','This will recalculate stock and fuel costs from the remaining entries.','Delete purchase'))return;
    try {
      await api('pellets/purchases/'+entry.id,{method:'DELETE'});
      if(token===generation) {
        purchase.deleted(entry.id);
        toast('Pellet purchase deleted.');
        await load(true);
      }
    } catch(error) {
      if(token===generation)toast(error.message);
    }
  }

  async function saveSettings(value) {
    await api('pellets/settings',{method:'PUT',body:JSON.stringify(value)});
    if(token===generation) {
      toast('Pellet supply settings saved.');
      await load(true);
    }
  }
}

function pelletPeriodLabel(kind,from) {
  const date = new Date(from);
  const formats = {
    hour:{month:'short',day:'numeric',hour:'numeric',minute:'2-digit',timeZoneName:'short'},
    day:{dateStyle:'medium'}, week:{dateStyle:'medium'},
    month:{month:'long',year:'numeric'}, year:{year:'numeric'},
  };
  const label = date.toLocaleString(undefined,formats[kind]);
  return kind==='week' ? 'Week of '+label : label;
}

function pelletTable(headings,rows) {
  return dom('div',{class:'table-scroll'},dom('table',{},
    dom('thead',{},dom('tr',{},headings.map(label=>dom('th',{scope:'col'},label)))),
    dom('tbody',{},rows.map(([name,...cells])=>
      dom('tr',{},dom('th',{scope:'row'},name),cells.map(cell=>dom('td',{},cell)))))));
}

function pelletField(label,input) {
  return dom('div',{class:'pellet-field'},dom('label',{for:input.id},label),input);
}

function pelletUnitSelect(id) {
  return dom('select',{id,class:'field-input'},
    dom('option',{value:'kg'},'Kilograms (kg)'),dom('option',{value:'lb'},'Pounds (lb)'));
}

function pelletNumberInput(id,options) {
  return dom('input',{id,type:'number',class:'field-input',step:'any',...options});
}

function pelletPurchaseForm(save) {
  const date = dom('input',{id:'pellet-purchased',type:'datetime-local',class:'field-input',required:true});
  const brand = dom('input',{id:'pellet-brand',class:'field-input',maxlength:60,placeholder:'Optional'});
  const quantity = pelletNumberInput('pellet-quantity',{min:0.001,max:1000000,required:true});
  const weight = pelletNumberInput('pellet-weight',{min:0.001,max:1000000,required:true});
  const unit = pelletUnitSelect('pellet-unit');
  const price = pelletNumberInput('pellet-price',{min:0,max:1000000000,placeholder:'Optional'});
  const priceLabel = dom('span');
  const error = dom('p',{class:'pellet-error',role:'alert'});
  const submit = dom('button',{class:'primary',type:'submit'},'Add purchase');
  const cancel = btn('Cancel edit',()=>reset(),'secondary',{hidden:true});
  const fields = dom('fieldset',{class:'pellet-form',disabled:true},
    dom('div',{class:'pellet-inputs'},pelletField('Purchase date and time',date),pelletField('Brand',brand),
      pelletField('Number bought',quantity),pelletField('Weight each',weight),pelletField('Weight unit',unit),pelletField(priceLabel,price)),
    dom('p',{class:'muted'},'Enter any size, such as one 2,000 lb pallet or ten 40 lb bags. For stock already on hand, enter today’s remaining weight and leave the price blank if unknown.'),
    error,dom('div',{class:'pellet-actions'},submit,cancel));
  let draftID;
  let editing;
  let draftAt;
  let saving = false;
  const form = dom('form',{onsubmit:async event=>{
    event.preventDefault();
    if(saving)return;
    const at = date.value===localDateInput(draftAt) ? draftAt : new Date(date.value);
    if(!Number.isFinite(at.getTime()))return;
    saving = true;
    fields.disabled = true;
    error.textContent = '';
    try {
      await save(draftID,{at:at.toISOString(),brand:brand.value,quantity:quantity.valueAsNumber,
        weight:weight.valueAsNumber,unit:unit.value,price:price.value===''?null:price.valueAsNumber});
      reset();
    } catch(e) {
      error.textContent = e.message;
    } finally {
      saving = false;
      fields.disabled = false;
    }
  }},fields);

  function localDateInput(at) {
    const local = new Date(at);
    return new Date(local.getTime()-local.getTimezoneOffset()*60000).toISOString().slice(0,16);
  }

  function reset() {
    draftID = Array.from(crypto.getRandomValues(new Uint8Array(16)),v=>v.toString(16).padStart(2,'0')).join('');
    editing = false;
    draftAt = new Date(Math.floor(Date.now()/1000)*1000);
    date.value = localDateInput(draftAt);
    brand.value = '';
    quantity.value = 1;
    weight.value = '';
    price.value = '';
    error.textContent = '';
    submit.textContent = 'Add purchase';
    cancel.hidden = true;
  }
  reset();
  return {
    form,
    ready(currency) {
      priceLabel.textContent = 'Price each ('+currency+')';
      if(!saving)fields.disabled = false;
    },
    edit(entry) {
      if(saving)return;
      draftID = entry.id;
      editing = true;
      draftAt = new Date(entry.at);
      date.value = localDateInput(draftAt);
      brand.value = entry.brand;
      quantity.value = entry.quantity;
      weight.value = entry.weight;
      unit.value = entry.unit;
      price.value = entry.price??'';
      error.textContent = '';
      submit.textContent = 'Save purchase';
      cancel.hidden = false;
      date.focus();
    },
    deleted(id) {
      if(!saving&&editing&&draftID===id)reset();
    },
  };
}

function pelletSupplyForm(save) {
  const currency = dom('input',{id:'pellet-currency',class:'field-input',required:true,maxlength:3,pattern:'[A-Za-z]{3}',placeholder:'USD'});
  const threshold = pelletNumberInput('pellet-threshold',{min:0,max:1000000,placeholder:'Off'});
  const unit = pelletUnitSelect('pellet-threshold-unit');
  const error = dom('p',{class:'pellet-error',role:'alert'});
  const submit = dom('button',{type:'submit',class:'secondary'},'Save supply settings');
  const fields = dom('fieldset',{class:'pellet-form',disabled:true},
    dom('div',{class:'pellet-inputs'},pelletField('Low-stock threshold',threshold),pelletField('Threshold unit',unit),
      pelletField('Currency code',currency)),
    dom('p',{class:'muted'},'Leave the threshold blank to turn off low-stock notices. Enable the pellet supply category for desktop delivery. ',
      dom('a',{href:'#events/notifications'},'Desktop notifications')),
    error,dom('div',{class:'pellet-actions'},submit));
  let dirty = false;
  let saving = false;
  const form = dom('form',{oninput:()=>{dirty=true;},onsubmit:async event=>{
    event.preventDefault();
    if(saving)return;
    saving = true;
    fields.disabled = true;
    error.textContent = '';
    dirty = false;
    const value = {currency:currency.value,lowWeight:threshold.value===''?0:threshold.valueAsNumber,lowUnit:unit.value};
    try {
      await save(value);
    } catch(e) {
      dirty = true;
      error.textContent = e.message;
    } finally {
      saving = false;
      fields.disabled = false;
    }
  }},fields);
  return {
    form,
    ready(settings,locked) {
      currency.readOnly = locked;
      if(!saving)fields.disabled = false;
      if(dirty)return;
      currency.value = settings.currency;
      threshold.value = settings.lowWeight||'';
      unit.value = settings.lowUnit;
    },
  };
}
