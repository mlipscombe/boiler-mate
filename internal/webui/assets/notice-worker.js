'use strict';

self.addEventListener('push',event=>{
  let notice={};
  try {notice=event.data?.json() ?? {};} catch {}
  event.waitUntil(self.registration.showNotification('Boiler Mate',{
    body:notice.body ?? 'A new boiler notice is available.',
    icon:'/icon-192.png',
    tag:notice.id==null ? 'boiler' : 'boiler-'+notice.id,
  }));
});

self.addEventListener('notificationclick',event=>{
  event.notification.close();
  const url=new URL('/#events',self.location.origin).href;
  event.waitUntil((async()=>{
    const windows=await self.clients.matchAll({type:'window',includeUncontrolled:true});
    const tab=windows.find(client=>client.url===url);
    if(tab)await tab.focus();
    else await self.clients.openWindow(url);
  })());
});
