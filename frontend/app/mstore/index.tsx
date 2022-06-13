import React from 'react';
import DashboardStore, { IDashboardSotre } from './dashboardStore';
import MetricStore, { IMetricStore } from './metricStore';
import UserStore from './userStore';
import RoleStore from './roleStore';
import APIClient from 'App/api_client';
import FunnelStore from './funnelStore';
import { makeAutoObservable, observable, action } from "mobx"
import { dashboardService, metricService, sessionService, userService, auditService, funnelService, errorService } from 'App/services';
import SettingsStore from './settingsStore';
import AuditStore from './auditStore';
import NotificationStore from './notificationStore';
import ErrorStore from './errorStore';

export class RootStore {
    dashboardStore: IDashboardSotre;
    metricStore: IMetricStore;
    funnelStore: FunnelStore;
    settingsStore: SettingsStore;
    userStore: UserStore;
    roleStore: RoleStore;
    auditStore: AuditStore;
    errorStore: ErrorStore;
    notificationStore: NotificationStore

    limits: any;

    constructor() {
        this.dashboardStore = new DashboardStore();
        this.metricStore = new MetricStore();
        this.funnelStore = new FunnelStore();
        this.settingsStore = new SettingsStore();
        this.userStore = new UserStore();
        this.roleStore = new RoleStore();
        this.auditStore = new AuditStore();
        this.errorStore = new ErrorStore();
        this.notificationStore = new NotificationStore();
        makeAutoObservable(this, {
          limits: observable,
          fetchLimits: action,
        });
    }

    initClient() {
      const client  = new APIClient();
      dashboardService.initClient(client)
      metricService.initClient(client)
      funnelService.initClient(client)
      sessionService.initClient(client)
      userService.initClient(client)
      auditService.initClient(client)
      errorService.initClient(client)
    }

    fetchLimits(): Promise<any> {
      return new Promise((resolve, reject) => {
          userService.getLimits()
              .then((response: any) => {
                  this.limits = response;
                  resolve(response);
              }).catch((error: any) => {
                  reject(error);
              });
      });
    }
}

const StoreContext = React.createContext<RootStore>({} as RootStore);

export const StoreProvider = ({ children, store }: any) => {
  return (
    <StoreContext.Provider value={store}>{children}</StoreContext.Provider>
  );
};

export const useStore = () => React.useContext(StoreContext);

export const withStore = (Component: any) => (props: any) => {
  return <Component {...props} mstore={useStore()} />;
};
