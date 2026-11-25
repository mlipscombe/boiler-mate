'use strict';

const controllerSections = {
  charts: [['recorded','Boiler Mate history'],['chart','Controller history']],
  events: [['activity','Boiler Mate events'],['events','Controller log']],
  'live-data': [['all','All readings'],['values','Selected readings']],
  system: [['controller','Controller settings'],['versions','Firmware versions'],['sensors','Wireless sensors']],
};

function controllerTabs(pageID, section) {
  return renderSections(pageID,controllerSections[pageID],section,'Data source');
}

function readTable(headers, rows, emptyMessage) {
  if (!rows.length) return empty(emptyMessage);
  return dom('div',{class:'table-scroll'},dom('table',{},
    dom('thead',{},dom('tr',{},headers.map(header=>dom('th',{scope:'col'},header)))),
    dom('tbody',{},rows.map(cells=>dom('tr',{},cells.map((cell,i)=>
      i ? dom('td',{},display(cell)) : dom('th',{scope:'row'},display(cell))))))));
}

function localDateInput(date) {
  const local = new Date(date.valueOf()-date.getTimezoneOffset()*60000);
  return local.toISOString().slice(0,16);
}

function controllerReadPage(pageID, kind, token) {
  const titles = {versions:'Firmware versions',sensors:'Wireless room sensors',events:'Controller event log',
    chart:'Controller chart history',values:'Selected operating readings'};
  const descriptions = {versions:'Versions offered by this controller.',events:'Read the boiler’s own log around a chosen time.',
    sensors:'Read the wireless room sensors paired with this controller.',
    values:'Choose the operating values to read together from the controller.',
    chart:'Enter the numeric chart IDs used by your controller and select a period within one controller day.'};
  const fields = dom('div',{class:'toolbar'});
  const output = dom('div',{'aria-live':'polite'});
  const now = new Date();
  const input = (label,type,value,attrs={})=>{
    const node = dom('input',{type,value,...attrs});
    fields.append(dom('label',{},label,node));
    return node;
  };
  let params = ()=>new URLSearchParams();
  let render;
  if (kind==='versions') {
    render = rows=>output.replaceChildren(readTable(['Version','Release date','Release type'],
      rows.map(r=>[r.version,/^\d{6}$/.test(r.date)?'20'+r.date.slice(0,2)+'-'+r.date.slice(2,4)+'-'+r.date.slice(4):r.date,r.releaseType]),
      'The controller listed no firmware versions.'));
  } else if (kind==='sensors') {
    render = rows=>output.replaceChildren(readTable(['Sensor','Temperature','Battery','Reading age (s)','Timeout'],
      rows.map(r=>[r.mac,r.temperature,r.battery,r.secondsSinceReading,r.timeout]),
      'The controller listed no wireless room sensors.'));
  } else if (kind==='events') {
    const at = input('Around','datetime-local','');
    params = ()=>at.value ? new URLSearchParams({at:Math.floor(new Date(at.value).valueOf()/1000)}) : new URLSearchParams();
    render = rows=>output.replaceChildren(readTable(['Time','Event type','Fields','Unit'],
      rows.map(r=>[r.time?new Date(r.time*1000).toLocaleString():r.date+' '+r.clock,r.type,r.fields.join(' · '),r.unit]),
      'The controller returned no events for this time.'));
  } else if (kind==='values') {
    const choices = Object.keys(snapshot('operating_data')).filter(name=>!['state_on','state_text'].includes(name)).sort();
    const select = dom('select',{multiple:true,size:6,required:true},choices.map(name=>
      dom('option',{value:name,selected:['boiler_temp','boiler_ref','state'].includes(name)},textLabel(name)+' ('+name+')')));
    fields.append(dom('label',{},'Operating values',select));
    params = ()=>new URLSearchParams({names:Array.from(select.selectedOptions,o=>o.value).join(',')});
    render = data=>output.replaceChildren(readTable(['Signal','Controller reading'],
      Object.entries(data).map(([name,value])=>[textLabel(name),value]),'No readings returned.'));
  } else {
    const start = new Date(Math.max(new Date(now).setHours(0,0,0,0),now.valueOf()-3600000));
    const from = input('From','datetime-local',localDateInput(start),{required:true});
    const to = input('To','datetime-local',localDateInput(now),{required:true});
    const ids = input('Chart IDs','text','0',{required:true,placeholder:'0, 3, 7'});
    const step = input('Step (minutes)','number','5',{min:1,max:1440,required:true});
    params = ()=>{
      const idsText = ids.value.split(',').map(s=>s.trim()).join(',');
      if(!/^\d+(,\d+)*$/.test(idsText)) throw new Error('Enter chart IDs separated by commas.');
      const sorted = [...new Set(idsText.split(',').map(Number))].sort((a,b)=>a-b);
      if(sorted.length>8) throw new Error('Choose up to eight chart IDs.');
      return new URLSearchParams({from:Math.floor(new Date(from.value).valueOf()/1000),
        to:Math.floor(new Date(to.value).valueOf()/1000),ids:sorted.join(','),step:step.value});
    };
    const graph = dom('div');
    render = (rows,query)=>{
      const ids = query.get('ids').split(',');
      const data = Object.fromEntries(ids.map((id,i)=>['Chart '+id,rows.filter(r=>r.time)
        .map(r=>({time:r.time,value:r.values[i]})).sort((a,b)=>a.time-b.time)]));
      graph.replaceChildren();
      chartRenderers.delete(graph);
      output.replaceChildren(graph,readTable(['Time',...Object.keys(data)],rows.map(r=>[
        r.time?new Date(r.time*1000).toLocaleString():r.rawTime,...r.values]),'No controller samples in this period.'));
      if(rows.some(r=>r.time)) showChart(graph,data,Number(query.get('to'))-Number(query.get('from')),
        {endTime:Number(query.get('to')),names:Object.fromEntries(Object.keys(data).map(k=>[k,k]))});
    };
  }
  const submit = dom('button',{type:'submit',class:'secondary'},
    kind==='chart'?'Read history':kind==='values'?'Read selected values':'Refresh');
  const form = dom('form',{},fields,submit);
  const feedback = dom('div',{'role':'status'});
  let haveFirmwareList = false;
  async function load(manual=false) {
    if(!form.reportValidity())return;
    submit.disabled=true;
    if(!haveFirmwareList)output.replaceChildren();
    feedback.replaceChildren(empty('Reading from the controller…'));
    try {
      const query=params();
      if(manual&&kind==='versions')query.set('refresh','1');
      const rows=await api('controller/'+kind+'?'+query);
      if(token===generation){
        render(rows,query);
        haveFirmwareList=kind==='versions';
        feedback.replaceChildren();
      }
    } catch(error) {
      if(token===generation)feedback.replaceChildren(empty(haveFirmwareList?
        'Refresh failed. Showing the previous firmware list. '+error.message:error.message));
    } finally {submit.disabled=false;}
  }
  form.addEventListener('submit',event=>{event.preventDefault();load(true);});
  main.replaceChildren(heading(titles[kind],descriptions[kind]),dom('div',{class:'stack'},
    controllerTabs(pageID,kind),dom('section',{class:'card'},form,feedback,output)));
  if(['versions','sensors','events'].includes(kind))load();
}
