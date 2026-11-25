'use strict';

const efficiencySections = [['suggestions','Suggestions'],['weather','Weather & fuel']];
const degreePeriods = [
  ['hour','Hourly', {month:'short',day:'numeric',hour:'numeric',timeZoneName:'short'}, 65*60],
  ['day','Daily', {month:'short',day:'numeric'}, 26*3600],
  ['week','Weekly', {month:'short',day:'numeric'}, 8*86400],
  ['month','Monthly', {month:'short',year:'numeric'}, 32*86400],
];

function degreeWeatherPage(token) {
  const preferences=saved('degreeWeather',{period:'day',unit:'F',baseC:(65-32)*5/9});
  let shownUnit=preferences.unit==='C'?'C':'F';
  const period=dom('select',{},degreePeriods.map(([id,label])=>
    dom('option',{value:id,selected:id===preferences.period},label)));
  const unit=dom('select',{},[['F','°F'],['C','°C']].map(([id,label])=>
    dom('option',{value:id,selected:id===shownUnit},label)));
  const base=dom('input',{type:'number',step:'any',required:true,size:4});
  function baseInC() {
    return shownUnit==='F'?(base.valueAsNumber-32)*5/9:base.valueAsNumber;
  }
  function setBase(celsius) {
    base.min=shownUnit==='F'?32:0;
    base.max=shownUnit==='F'?86:30;
    base.value=(shownUnit==='F'?celsius*9/5+32:celsius).toFixed(1);
  }
  setBase(preferences.baseC);
  unit.addEventListener('change',()=>{
    const celsius=baseInC();
    shownUnit=unit.value;
    setBase(Number.isFinite(celsius)?celsius:preferences.baseC);
  });

  const output=dom('div',{class:'stack'});
  let lastRequest=0;
  let chartTargets=[];
  async function load() {
    const request=++lastRequest;
    const periodID=period.value;
    const params=new URLSearchParams({period:periodID,unit:shownUnit,base:base.value,
      tz:Intl.DateTimeFormat().resolvedOptions().timeZone});
    saveLocal('degreeWeather',{period:periodID,unit:shownUnit,baseC:baseInC()});
    chartTargets.forEach(target=>chartRenderers.delete(target));
    chartTargets=[];
    output.replaceChildren(empty('Loading weather and fuel history…'));
    try {
      const report=await api('efficiency/weather?'+params);
      if (token!==generation || request!==lastRequest) return;
      const available=report.periods.findLastIndex(row=>row.coverage>0 || row.usedKg!=null);
      if (available<0) {
        output.replaceChildren(card('Heating degree history',null,
          empty('Waiting for outdoor readings. Select a local sensor in External heating to use it ahead of boiler weather.')));
        return;
      }
      const rows=report.periods.slice(0,available+1);
      const hourly=periodID==='hour';
      const degreeUnit='°'+report.unit;
      const [,,dateOptions,maxGap]=degreePeriods.find(([id])=>id===periodID);
      const dateFormat=new Intl.DateTimeFormat(undefined,dateOptions);
      const money=new Intl.NumberFormat(undefined,{style:'currency',currency:report.currency});
      const sources={local:'Local MQTT',remote:'Boiler weather',mixed:'Local + boiler'};
      function chart(title, values) {
        const points=rows.filter(row=>row.complete).toReversed().flatMap(row=>{
          const value=values(row);
          return value==null?[]:[{time:Date.parse(row.from)/1000,value}];
        });
        if (points.length<2) return null;
        const plot=dom('div');
        const first=points[0].time, last=points.at(-1).time;
        chartTargets.push(plot);
        showChart(plot,{trend:points},last-first,{names:{trend:title},maxGap,endTime:last});
        return card(title,null,plot);
      }
      const weatherTrend=chart(hourly?'Heating degree hours':'Equivalent heating degree days',
        row=>row.coverage>=.95?(hourly?row.degreeHours:row.degreeDays):null);
      const fuelTrend=chart('Pellets per equivalent degree day',row=>row.kgPerDegreeDay);
      const history=readTable(
        ['Period','Degree hours','Equivalent days','Weather coverage','Source','Pellets (kg)','Joint coverage','kg per degree day','Fuel cost','Cost per degree day','Pellet brand'],
        rows.map(row=>[
          dateFormat.format(new Date(row.from))+(row.complete?'':' · so far'),
          row.coverage?decimalTenths.format(row.degreeHours):null,
          row.coverage?decimalTenths.format(row.degreeDays):null,
          numeric.format(row.coverage*100)+'%',
          sources[row.source],
          row.usedKg,numeric.format(row.fuelCoverage*100)+'%',row.kgPerDegreeDay,
          row.cost==null?null:money.format(row.cost),
          row.costPerDegreeDay==null?null:money.format(row.costPerDegreeDay),
          row.brand || (row.mixedBrands?'Mixed':null),
        ]),'Waiting for observations.');
      const brands=report.brands.length?card('Pellet brand comparison',
        'Includes completed periods attributed to one brand by the purchase ledger.',
        readTable(['Brand','Periods','Pellets (kg)','Equivalent days','kg per degree day'],
          report.brands.map(row=>[row.name,row.periods,row.usedKg,row.degreeDays,row.kgPerDegreeDay]),'')):null;
      output.replaceChildren(...[
        (weatherTrend || fuelTrend) && dom('div',{class:weatherTrend && fuelTrend?'grid split':'stack'},weatherTrend,fuelTrend),
        card('Weather and pellet history',
          'Base '+display(report.base)+' '+degreeUnit+'. Equivalent days are the observed degree hours divided by 24.',
          history,
          dom('p',{class:'muted'},'Fuel comparisons use completed periods with at least 95% overlap between weather and pellet readings. Pellet use includes any hot-water production. Brand attribution follows purchase order.')),
        brands,
      ].filter(Boolean));
    } catch(error) {
      if (token===generation && request===lastRequest) output.replaceChildren(empty(error.message));
    }
  }

  const controls=dom('form',{class:'toolbar',onsubmit:event=>{event.preventDefault();load();}},
    dom('label',{},'Interval',period),dom('label',{},'Heating base',base),dom('label',{},'Units',unit),
    btn('Update',null,'secondary',{type:'submit'}));
  main.replaceChildren(
    heading('Weather & fuel','Compare pellet use across periods with different outdoor heating demand.'),
    dom('div',{class:'stack'},renderSections('efficiency',efficiencySections,'weather','Efficiency views'),
      card('Report settings',null,controls),output));
  load();
}
